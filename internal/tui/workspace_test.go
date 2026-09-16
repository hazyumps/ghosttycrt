package tui_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hazyumps/ghosttycrt/internal/config"
	"github.com/hazyumps/ghosttycrt/internal/session"
	"github.com/hazyumps/ghosttycrt/internal/tmux"
	"github.com/hazyumps/ghosttycrt/internal/tui"
)

// workspaceModel builds the shape bootstrapWorkspace creates — a tmux session
// on a private socket with the tree pane already in it — and points the ssh
// transport at a script, so a "connection" stays up instead of really dialling.
func workspaceModel(t *testing.T, width int, sshBody string) (*tui.Model, *tmux.Client) {
	return workspaceLayout(t, config.WorkspaceSplit, width, sshBody)
}

func workspaceLayout(t *testing.T, layout string, width int, sshBody string) (*tui.Model, *tmux.Client) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}

	script := filepath.Join(t.TempDir(), "fake-ssh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"+sshBody+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	socket := fmt.Sprintf("gcrt-ws-%d-%s", os.Getpid(), layout)
	client := tmux.New(socket)
	t.Cleanup(func() { exec.Command("tmux", "-L", socket, "kill-server").Run() })

	out, err := exec.Command("tmux", "-L", socket, "new-session", "-d",
		"-s", tmux.WorkspaceSession, "-n", tmux.TreeWindow, "sleep", "300").CombinedOutput()
	if err != nil {
		t.Fatalf("bootstrapping the workspace: %v: %s", err, out)
	}

	cfg := config.Default()
	cfg.General.WorkspaceLayout = layout
	cfg.General.WorkspaceTreeWidth = 30
	cfg.Transports.SSH.Binary = script

	m := tui.New(cfg, sample(), config.DefaultPaths(), session.Problems{}, client)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
	m = updated.(*tui.Model)
	m.EnableWorkspace("%0")
	return m, client
}

func enterKey(t *testing.T, m *tui.Model) (*tui.Model, tea.Cmd) {
	t.Helper()
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	return updated.(*tui.Model), cmd
}

func mustOpen(t *testing.T, m *tui.Model) *tui.Model {
	t.Helper()
	m, cmd := enterKey(t, m)
	if cmd == nil {
		t.Fatal("enter produced no command")
	}
	updated, _ := m.Update(cmd())
	return updated.(*tui.Model)
}

// enterAndPump runs whatever command the current selection produced, if any.
// Menu items that open a session return one; Hide and Shutdown do not.
func enterAndPump(t *testing.T, m *tui.Model) *tui.Model {
	t.Helper()
	m, cmd := enterKey(t, m)
	if cmd == nil {
		return m
	}
	updated, _ := m.Update(cmd())
	return updated.(*tui.Model)
}

func paneForSlug(t *testing.T, c *tmux.Client, slug string) (tmux.Pane, bool) {
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

func TestWorkspaceEnterOpensTheSessionBesideTheTree(t *testing.T) {
	m, client := workspaceModel(t, 34, "sleep 60")
	m = selectK3s(t, m)
	m = mustOpen(t, m)

	p, ok := paneForSlug(t, client, "k3s-01")
	if !ok {
		t.Fatal("k3s-01 has no pane in the workspace")
	}
	if !p.Visible() {
		t.Fatalf("pane is not tiled beside the tree: %+v", p)
	}

	out := m.View()
	if !strings.Contains(out, "●") {
		t.Errorf("expected the open glyph:\n%s", out)
	}
	if !strings.Contains(out, "open beside the tree") {
		t.Errorf("expected a status line saying it opened:\n%s", out)
	}
}

func TestWorkspaceEnterIsIdempotent(t *testing.T) {
	m, client := workspaceModel(t, 34, "sleep 60")
	m = selectK3s(t, m)
	m = mustOpen(t, m)
	first, _ := paneForSlug(t, client, "k3s-01")

	m = mustOpen(t, m)
	second, _ := paneForSlug(t, client, "k3s-01")
	if first.ID != second.ID {
		t.Fatalf("second enter made a new pane: %s then %s", first.ID, second.ID)
	}

	panes, _ := client.Panes()
	if len(panes) != 2 { // the tree pane plus one session
		t.Fatalf("panes = %d, want 2", len(panes))
	}
}

// A wide tree pane still shows the details column; the narrow default does not.
func TestWorkspaceWideShowsDetails(t *testing.T) {
	m, _ := workspaceModel(t, 120, "sleep 60")
	m = selectK3s(t, m)
	m = mustOpen(t, m)

	if out := m.View(); !strings.Contains(out, "open beside the tree") {
		t.Errorf("details should report the session as open:\n%s", out)
	}
}

func TestWorkspaceMenuHidesAndKeepsItRunning(t *testing.T) {
	m, client := workspaceModel(t, 34, "sleep 60")
	m = selectK3s(t, m)
	m = mustOpen(t, m)

	m, _ = press(t, m, "d")
	if out := m.View(); !strings.Contains(out, "Hide") {
		t.Fatalf("menu should offer Hide for an open session:\n%s", out)
	}
	m = enterAndPump(t, m)

	p, ok := paneForSlug(t, client, "k3s-01")
	if !ok {
		t.Fatal("hiding removed the pane instead of backgrounding it")
	}
	if p.Visible() {
		t.Fatal("pane is still tiled after Hide")
	}
	if out := m.View(); !strings.Contains(out, "hidden") {
		t.Errorf("expected a status line saying it is still running:\n%s", out)
	}
}

func TestWorkspaceMenuOffersToShowAgain(t *testing.T) {
	m, _ := workspaceModel(t, 34, "sleep 60")
	m = selectK3s(t, m)
	m = mustOpen(t, m)

	m, _ = press(t, m, "d")
	m = enterAndPump(t, m) // Hide

	m, _ = press(t, m, "d")
	if out := m.View(); !strings.Contains(out, "Show") {
		t.Fatalf("menu should offer Show for a hidden session:\n%s", out)
	}
	m = enterAndPump(t, m) // Show

	if out := m.View(); !strings.Contains(out, "open beside the tree") {
		t.Errorf("session should be open again:\n%s", out)
	}
}

// A connection that dies must not just vanish: the pane stays, showing why.
func TestWorkspaceShowsACrashedConnection(t *testing.T) {
	m, _ := workspaceModel(t, 120, "exit 3")
	m = selectK3s(t, m)
	m = mustOpen(t, m)

	deadline := 100
	for i := 0; i < deadline; i++ {
		if strings.Contains(m.View(), "exited (status 3)") {
			return
		}
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
		m = updated.(*tui.Model)
	}
	t.Fatalf("a crashed connection should be reported, not hidden:\n%s", m.View())
}

// q detaches in workspace mode: killing it would take every session with it.
func TestWorkspaceQuitDetachesInsteadOfQuitting(t *testing.T) {
	m, _ := workspaceModel(t, 34, "sleep 60")

	_, cmd := press(t, m, "q")
	if cmd != nil {
		t.Fatal("q should detach the workspace, not quit gcrt")
	}
}

func TestWorkspaceMenuOffersShutdown(t *testing.T) {
	m, _ := workspaceModel(t, 34, "sleep 60")
	m, _ = press(t, m, "d")
	if out := m.View(); !strings.Contains(out, "Shut down workspace") {
		t.Fatalf("menu should offer shutting the workspace down:\n%s", out)
	}
}

// An oversized modal used to spill out of the 34-column tree pane and blank the
// tree, because Place cannot shrink a box it is handed.
func TestWorkspaceModalsFitTheNarrowPane(t *testing.T) {
	const width = 34
	m, _ := workspaceModel(t, width, "sleep 60")
	m = selectK3s(t, m)
	m = mustOpen(t, m)

	check := func(state string) {
		t.Helper()
		for _, line := range strings.Split(m.View(), "\n") {
			if w := lipgloss.Width(line); w > width {
				t.Errorf("%s: line is %d columns in a %d-column pane: %q", state, w, width, line)
			}
		}
	}

	check("tree")

	m, _ = press(t, m, "d")
	check("menu")
	m = pressKey(t, m, tea.KeyEscape)

	m, _ = press(t, m, "?")
	check("help")
	m = pressKey(t, m, tea.KeyEscape)

	m, _ = press(t, m, "d")
	m, _ = press(t, m, "jj")
	m = pressKey(t, m, tea.KeyEnter)
	if out := m.View(); !strings.Contains(out, "Shut down the workspace?") {
		t.Fatalf("expected the shutdown confirmation:\n%s", out)
	}
	check("confirm")
}

func TestWorkspaceShutdownConfirmsAndKillsEverything(t *testing.T) {
	m, client := workspaceModel(t, 34, "sleep 60")
	m = selectK3s(t, m)
	m = mustOpen(t, m)

	m, _ = press(t, m, "d")
	m, _ = press(t, m, "jj") // Shut down workspace is the last item
	m = pressKey(t, m, tea.KeyEnter)

	if out := m.View(); !strings.Contains(out, "Shut down the workspace?") {
		t.Fatalf("expected a confirmation:\n%s", out)
	}

	m, cmd := press(t, m, "y")
	if cmd == nil {
		t.Fatal("confirmed shutdown should quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("confirmed shutdown should return tea.Quit")
	}
	panes, _ := client.Panes()
	if len(panes) != 0 {
		t.Fatalf("workspace still has panes: %+v", panes)
	}
}
