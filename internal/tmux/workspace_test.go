package tmux_test

import (
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hazyumps/ghosttycrt/internal/tmux"
)

func splitLines(s string) []string {
	var out []string
	for _, l := range strings.Split(strings.TrimSpace(s), "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

// wsClient sets up the same shape bootstrapWorkspace creates: one session with
// a window named "tree" holding the tree pane.
func wsClient(t *testing.T, treeWidth int) *tmux.Client {
	t.Helper()
	c := testClient(t)
	out, err := exec.Command("tmux", "-L", c.Socket,
		"new-session", "-d", "-s", tmux.WorkspaceSession, "-n", tmux.TreeWindow, "sleep", "300").CombinedOutput()
	if err != nil {
		t.Fatalf("creating workspace: %v: %s", err, out)
	}
	if err := c.Configure(treeWidth, ""); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	return c
}

func paneBySlug(t *testing.T, c *tmux.Client, slug string) (tmux.Pane, bool) {
	t.Helper()
	panes, err := c.Panes()
	if err != nil {
		t.Fatalf("Panes: %v", err)
	}
	for _, p := range panes {
		if p.Slug == slug {
			return p, true
		}
	}
	return tmux.Pane{}, false
}

func treePaneWidth(t *testing.T, c *tmux.Client) int {
	t.Helper()
	out, err := exec.Command("tmux", "-L", c.Socket, "list-panes", "-t",
		tmux.WorkspaceSession+":"+tmux.TreeWindow, "-F", "#{pane_width}").Output()
	if err != nil {
		t.Fatalf("list-panes: %v", err)
	}
	w := 0
	for _, f := range splitLines(string(out)) {
		if n := atoi(f); n > 0 {
			w = n
			break
		}
	}
	return w
}

func TestWorkspaceShowCreatesAndTilesBesideTheTree(t *testing.T) {
	c := wsClient(t, 30)

	id, err := c.Show("core-sw-01", []string{"sleep", "300"}, nil, 30)
	if err != nil {
		t.Fatalf("Show: %v", err)
	}

	p, ok := paneBySlug(t, c, "core-sw-01")
	if !ok {
		t.Fatal("the session is not visible in the workspace")
	}
	if !p.Visible() {
		t.Fatalf("pane window = %q, want %q", p.Window, tmux.TreeWindow)
	}
	if p.ID != id {
		t.Fatalf("pane id %q, Show reported %q", p.ID, id)
	}
	if w := treePaneWidth(t, c); w != 30 {
		t.Fatalf("tree pane width = %d, want 30 (main-vertical must pin it)", w)
	}
}

func TestWorkspaceShowIsIdempotent(t *testing.T) {
	c := wsClient(t, 30)

	first, err := c.Show("core-sw-01", []string{"sleep", "300"}, nil, 30)
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.Show("core-sw-01", []string{"sleep", "300"}, nil, 30)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("Show recreated the pane: %q then %q", first, second)
	}

	panes, err := c.Panes()
	if err != nil {
		t.Fatal(err)
	}
	if len(panes) != 2 { // the tree pane plus one session
		t.Fatalf("panes = %d, want 2", len(panes))
	}
}

func TestWorkspaceHideKeepsTheProcessAndShowBringsItBack(t *testing.T) {
	c := wsClient(t, 30)

	id, err := c.Show("core-sw-01", []string{"sleep", "300"}, nil, 30)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Hide(id, "core-sw-01"); err != nil {
		t.Fatalf("Hide: %v", err)
	}

	p, ok := paneBySlug(t, c, "core-sw-01")
	if !ok {
		t.Fatal("hiding destroyed the pane instead of backgrounding it")
	}
	if p.Visible() {
		t.Fatal("pane is still in the tree window after Hide")
	}
	if p.Window != "core-sw-01" {
		t.Fatalf("hidden window = %q, want the slug", p.Window)
	}
	if c.HasTarget(tmux.SessionName("core-sw-01")) {
		t.Error("Hide should not leave a gscrt/<slug> session behind")
	}

	back, err := c.Show("core-sw-01", []string{"sleep", "300"}, nil, 30)
	if err != nil {
		t.Fatal(err)
	}
	if back != id {
		t.Fatalf("Show after Hide made a new pane: %q vs %q", back, id)
	}
	if p, _ = paneBySlug(t, c, "core-sw-01"); !p.Visible() {
		t.Fatal("pane is not visible again")
	}
	if w := treePaneWidth(t, c); w != 30 {
		t.Fatalf("tree width = %d after re-show, want 30", w)
	}
}

func TestWorkspaceKillRemovesThePane(t *testing.T) {
	c := wsClient(t, 30)

	id, err := c.Show("core-sw-01", []string{"sleep", "300"}, nil, 30)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.KillPane(id); err != nil {
		t.Fatalf("KillPane: %v", err)
	}
	if _, ok := paneBySlug(t, c, "core-sw-01"); ok {
		t.Fatal("pane survived KillPane")
	}
}

// remain-on-exit means a crashed connection stays on screen with its status
// instead of silently vanishing.
func TestWorkspaceReportsADeadPaneAndItsExitStatus(t *testing.T) {
	c := wsClient(t, 30)

	if _, err := c.Show("dies", []string{"sh", "-c", "exit 7"}, nil, 30); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if p, ok := paneBySlug(t, c, "dies"); ok && p.Dead {
			if p.Exit != 7 {
				t.Fatalf("exit status = %d, want 7", p.Exit)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("pane never reported itself dead")
}

func TestWorkspaceTilesSeveralSessions(t *testing.T) {
	c := wsClient(t, 30)

	for _, slug := range []string{"a", "b", "c"} {
		if _, err := c.Show(slug, []string{"sleep", "300"}, nil, 30); err != nil {
			t.Fatalf("Show %s: %v", slug, err)
		}
	}
	panes, err := c.Panes()
	if err != nil {
		t.Fatal(err)
	}
	if len(panes) != 4 {
		t.Fatalf("panes = %d, want 4 (tree + 3)", len(panes))
	}
	if w := treePaneWidth(t, c); w != 30 {
		t.Fatalf("tree width = %d with three sessions tiled, want 30", w)
	}
}

func TestConfigureBindsAKeyBackToTheTree(t *testing.T) {
	c := testClient(t)
	if out, err := exec.Command("tmux", "-L", c.Socket,
		"new-session", "-d", "-s", tmux.WorkspaceSession, "-n", tmux.TreeWindow, "sleep", "300").CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}

	out, err := exec.Command("tmux", "-L", c.Socket, "list-panes", "-t",
		tmux.WorkspaceSession+":"+tmux.TreeWindow, "-F", "#{pane_id}").Output()
	if err != nil {
		t.Fatal(err)
	}
	tree := splitLines(string(out))[0]

	if err := c.Configure(30, tree); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	bind, err := exec.Command("tmux", "-L", c.Socket, "list-keys", "-T", "prefix").Output()
	if err != nil {
		t.Fatal(err)
	}
	want := regexp.MustCompile(`(?m)^bind-key\s+-T prefix\s+t\s+select-pane -t "?` + regexp.QuoteMeta(tree) + `"?`)
	if !want.Match(bind) {
		t.Fatalf("prefix t is not bound to the tree pane %s:\n%s", tree, bind)
	}
}
