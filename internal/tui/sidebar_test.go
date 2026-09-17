package tui_test

import (
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hazyumps/ghosttycrt/internal/config"
	"github.com/hazyumps/ghosttycrt/internal/tmux"
	"github.com/hazyumps/ghosttycrt/internal/tui"
)

func sidebarModel(t *testing.T, width int, sshBody string) (*tui.Model, *tmux.Client) {
	t.Helper()
	return workspaceLayout(t, config.WorkspaceSidebar, width, sshBody)
}

// selectSession walks the tree until it lands on name.
func selectSession(t *testing.T, m *tui.Model, name string) *tui.Model {
	t.Helper()
	if m.SelectedName() == name {
		return m
	}
	for i := 0; i < 60; i++ {
		m, _ = press(t, m, "j")
		if m.SelectedName() == name {
			return m
		}
	}
	for i := 0; i < 60; i++ {
		m, _ = press(t, m, "k")
		if m.SelectedName() == name {
			return m
		}
	}
	t.Fatalf("could not select %q, stuck on %q", name, m.SelectedName())
	return m
}

// tabWindows lists the tab server's windows as "name<TAB>active".
func tabWindows(t *testing.T, c *tmux.Client) []string {
	t.Helper()
	out, err := exec.Command("tmux", "-L", c.TabsSocket, "list-windows", "-t",
		tmux.TabsSession, "-F", "#{window_name}\t#{window_active}").CombinedOutput()
	if err != nil {
		// The tab server does not exist until a connection opens, and it goes
		// away again when the last one is killed.
		if strings.Contains(string(out), "no server running") || strings.Contains(string(out), "can't find session") {
			return nil
		}
		t.Fatalf("list-windows on the tab server: %v: %s", err, out)
	}
	var names []string
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if l != "" {
			names = append(names, l)
		}
	}
	return names
}

func currentTab(t *testing.T, c *tmux.Client) string {
	t.Helper()
	for _, n := range tabWindows(t, c) {
		if strings.HasSuffix(n, "\t1") {
			return strings.SplitN(n, "\t", 2)[0]
		}
	}
	return ""
}

func treeWindowPanes(t *testing.T, c *tmux.Client) []int {
	t.Helper()
	out, err := exec.Command("tmux", "-L", c.Socket, "list-panes", "-t",
		tmux.WorkspaceSession+":"+tmux.TreeWindow, "-F", "#{pane_width}").Output()
	if err != nil {
		t.Fatalf("list-panes: %v", err)
	}
	var widths []int
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if l != "" {
			n, _ := strconv.Atoi(strings.TrimSpace(l))
			widths = append(widths, n)
		}
	}
	return widths
}

func TestSidebarKeepsTheTreeBesideTheTabs(t *testing.T) {
	m, client := sidebarModel(t, 120, "sleep 60")
	m = selectK3s(t, m)
	m = mustOpen(t, m)

	widths := treeWindowPanes(t, client)
	if len(widths) != 2 {
		t.Fatalf("the tree window has %d panes, want the tree plus the content pane", len(widths))
	}
	if widths[0] != 30 {
		t.Fatalf("tree pane width = %d, want the configured 30", widths[0])
	}

	// The connection is a window in the second server, not a pane in the tree.
	if got := tabWindows(t, client); len(got) != 1 || !strings.HasPrefix(got[0], "k3s-01") {
		t.Fatalf("tab server windows = %v, want just k3s-01", got)
	}
	if _, ok := paneForSlug(t, client, "k3s-01"); !ok {
		t.Fatal("k3s-01 is not tagged in the tab server")
	}
}

// The tab bar is the second server's status line; at the bottom it would just
// be a status line, so the position is the whole point.
func TestSidebarTabBarIsAtTheTop(t *testing.T) {
	m, client := sidebarModel(t, 120, "sleep 60")
	// The tab server only exists once something is open.
	m = selectK3s(t, m)
	m = mustOpen(t, m)

	out, err := exec.Command("tmux", "-L", client.TabsSocket, "show-options", "-g",
		"status-position").Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(out)); got != "status-position top" {
		t.Fatalf("tab bar position = %q, want it at the top", got)
	}
}

func TestSidebarOpensASecondConnectionAsAnotherTab(t *testing.T) {
	m, client := sidebarModel(t, 120, "sleep 60")

	m = selectK3s(t, m)
	m = mustOpen(t, m)
	if got := tabWindows(t, client); len(got) != 1 {
		t.Fatalf("tab server windows = %v", got)
	}

	m = selectSession(t, m, "core-sw-01")
	m = mustOpen(t, m)

	if got := tabWindows(t, client); len(got) != 2 {
		t.Fatalf("tab server windows = %v, want two", got)
	}
	if got := currentTab(t, client); got != "core-sw-01" {
		t.Fatalf("current tab = %q, want the one just opened", got)
	}
	// Still one window on the outside: the tree never moves.
	if widths := treeWindowPanes(t, client); len(widths) != 2 {
		t.Fatalf("the tree window now has %d panes", len(widths))
	}
}

func TestSidebarMenuOffersSwitchAndKill(t *testing.T) {
	m, client := sidebarModel(t, 120, "sleep 60")

	m = selectK3s(t, m)
	m = mustOpen(t, m)
	m = selectSession(t, m, "core-sw-01")
	m = mustOpen(t, m)

	// k3s-01 is now a background tab.
	m = selectSession(t, m, "k3s-01")
	m, _ = press(t, m, "d")
	out := m.View()
	if !strings.Contains(out, "Switch to it") {
		t.Fatalf("a background tab should offer to be switched to:\n%s", out)
	}
	if strings.Contains(out, "Hide") || strings.Contains(out, "Show") {
		t.Errorf("hide/show make no sense for tabs:\n%s", out)
	}

	m = enterAndPump(t, m)
	if got := currentTab(t, client); got != "k3s-01" {
		t.Fatalf("current tab = %q, want k3s-01", got)
	}
}

func TestSidebarKillRemovesTheTab(t *testing.T) {
	m, client := sidebarModel(t, 120, "sleep 60")
	m = selectK3s(t, m)
	m = mustOpen(t, m)

	m, _ = press(t, m, "d")
	if out := m.View(); !strings.Contains(out, "Kill") {
		t.Fatalf("expected a Kill item:\n%s", out)
	}
	m = enterAndPump(t, m)
	if out := m.View(); !strings.Contains(out, "Kill k3s-01?") {
		t.Fatalf("expected a confirmation:\n%s", out)
	}
	m, _ = press(t, m, "y")

	if got := tabWindows(t, client); len(got) != 0 {
		t.Fatalf("tab server windows = %v, want none", got)
	}
}

func TestSidebarShutdownTearsDownBothServers(t *testing.T) {
	m, client := sidebarModel(t, 120, "sleep 60")
	m = selectK3s(t, m)
	m = mustOpen(t, m)

	m, _ = press(t, m, "d")
	m = selectLastMenuItem(t, m)
	m = enterAndPump(t, m)
	m, cmd := press(t, m, "y")
	if cmd == nil {
		t.Fatal("confirmed shutdown should quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("confirmed shutdown should return tea.Quit")
	}

	time.Sleep(250 * time.Millisecond)
	for _, socket := range []string{client.Socket, client.TabsSocket} {
		out, _ := exec.Command("tmux", "-L", socket, "list-sessions").CombinedOutput()
		text := string(out)
		if !strings.Contains(text, "no server running") && !strings.Contains(text, "no sessions") {
			t.Errorf("socket %s still has a server: %s", socket, text)
		}
	}
}

// selectLastMenuItem walks to the bottom of the open action menu.
func selectLastMenuItem(t *testing.T, m *tui.Model) *tui.Model {
	t.Helper()
	for i := 0; i < 10; i++ {
		m, _ = press(t, m, "j")
	}
	return m
}

// The sidebar has no pane borders, so nothing on screen shows which pane owns
// the keyboard. Opening a session hands it to the content pane, and the tree has
// to say so rather than look merely idle.
func TestSidebarSaysWhenTheKeyboardIsElsewhere(t *testing.T) {
	m, client := sidebarModel(t, 120, "sleep 60")
	m = selectK3s(t, m)
	m = mustOpen(t, m) // Show focuses the content pane

	refresh := func() {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
		m = updated.(*tui.Model)
	}
	refresh()
	if out := m.View(); !strings.Contains(out, "Ctrl-b t") {
		t.Fatalf("the tree should say where the keys went:\n%s", out)
	}

	// Give the tree the keyboard back; the hint should go.
	tree, err := exec.Command("tmux", "-L", client.Socket, "list-panes", "-t",
		tmux.WorkspaceSession+":"+tmux.TreeWindow, "-F", "#{pane_id}").Output()
	if err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("tmux", "-L", client.Socket, "select-pane",
		"-t", strings.Fields(string(tree))[0]).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	refresh()
	if out := m.View(); strings.Contains(out, "keys are in the session pane") {
		t.Fatalf("the hint should clear once the tree has focus:\n%s", out)
	}
}
