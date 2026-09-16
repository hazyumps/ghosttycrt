package tui_test

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hazyumps/ghosttycrt/internal/config"
	"github.com/hazyumps/ghosttycrt/internal/tmux"
	"github.com/hazyumps/ghosttycrt/internal/tui"
)

func tabModel(t *testing.T, width int, sshBody string) (*tui.Model, *tmux.Client) {
	t.Helper()
	return workspaceLayout(t, config.WorkspaceTabs, width, sshBody)
}

func windows(t *testing.T, c *tmux.Client) []string {
	t.Helper()
	out, err := exec.Command("tmux", "-L", c.Socket, "list-windows", "-t",
		tmux.WorkspaceSession, "-F", "#{window_name}\t#{window_active}").Output()
	if err != nil {
		t.Fatalf("list-windows: %v", err)
	}
	var names []string
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if l != "" {
			names = append(names, l)
		}
	}
	return names
}

func currentWindow(t *testing.T, c *tmux.Client) string {
	t.Helper()
	out, err := exec.Command("tmux", "-L", c.Socket, "display-message", "-p", "#{window_name}").Output()
	if err != nil {
		t.Fatalf("display-message: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func TestTabsEnterOpensTheSessionAsATab(t *testing.T) {
	m, client := tabModel(t, 34, "sleep 60")
	m = selectK3s(t, m)
	m = mustOpen(t, m)

	p, ok := paneForSlug(t, client, "k3s-01")
	if !ok {
		t.Fatal("k3s-01 has no pane in the workspace")
	}
	if p.Window != "k3s-01" {
		t.Fatalf("pane is in window %q, want its own window", p.Window)
	}

	// The tree keeps a window of its own, and the session becomes the current one.
	names := windows(t, client)
	if len(names) != 2 {
		t.Fatalf("windows = %v, want the tree plus one session", names)
	}
	if got := currentWindow(t, client); got != "k3s-01" {
		t.Fatalf("current window = %q, want the session tab to be selected", got)
	}
}

func TestTabsEnterTwiceSelectsTheSameTab(t *testing.T) {
	m, client := tabModel(t, 34, "sleep 60")
	m = selectK3s(t, m)
	m = mustOpen(t, m)
	first, _ := paneForSlug(t, client, "k3s-01")

	m = mustOpen(t, m)
	second, _ := paneForSlug(t, client, "k3s-01")
	if first.ID != second.ID {
		t.Fatalf("second enter made a new tab: %s then %s", first.ID, second.ID)
	}
	if names := windows(t, client); len(names) != 2 {
		t.Fatalf("windows = %v, want no duplicate tab", names)
	}
}

func TestTabsShowAndHideBecomeSwitchToIt(t *testing.T) {
	m, client := tabModel(t, 34, "sleep 60")
	m = selectK3s(t, m)
	m = mustOpen(t, m)

	// Back to the tree: the session is now a background tab.
	if _, err := exec.Command("tmux", "-L", client.Socket, "select-window", "-t",
		tmux.WorkspaceSession+":"+tmux.TreeWindow).Output(); err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = updated.(*tui.Model)

	m, _ = press(t, m, "d")
	out := m.View()
	if !strings.Contains(out, "Switch to it") {
		t.Fatalf("tabs menu should offer to switch to the tab:\n%s", out)
	}
	if strings.Contains(out, "Hide") || strings.Contains(out, "Show") {
		t.Errorf("hide/show make no sense for tabs:\n%s", out)
	}

	m = enterAndPump(t, m)
	if got := currentWindow(t, client); got != "k3s-01" {
		t.Fatalf("current window = %q, want the session tab", got)
	}
}

func TestTabsStatusDescribesTheTab(t *testing.T) {
	m, client := tabModel(t, 34, "sleep 60")
	m = selectK3s(t, m)
	m = mustOpen(t, m)

	// While the tree window is front, the session is a background tab.
	if _, err := exec.Command("tmux", "-L", client.Socket, "select-window", "-t",
		tmux.WorkspaceSession+":"+tmux.TreeWindow).Output(); err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = updated.(*tui.Model)

	// A wide pane shows the details column, where the state line lives.
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = updated.(*tui.Model)

	if out := m.View(); !strings.Contains(out, "running in a tab") {
		t.Fatalf("details should describe a background tab:\n%s", out)
	}
	if out := m.View(); !strings.Contains(out, "○") {
		t.Errorf("a background tab should use the running glyph:\n%s", out)
	}
}

func TestTabsAGoneConnectionIsMarkedAndCanBeRestarted(t *testing.T) {
	// Exits 5 the first time, then stays up, so "restart" is observable.
	marker := filepath.Join(t.TempDir(), "first-run-done")
	body := fmt.Sprintf("if [ ! -f %s ]; then touch %s; exit 5; fi\nsleep 60", marker, marker)
	m, client := tabModel(t, 120, body)
	m = selectK3s(t, m)
	m = mustOpen(t, m)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(m.View(), "exited (status 5)") {
			break
		}
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
		m = updated.(*tui.Model)
		time.Sleep(50 * time.Millisecond)
	}
	if out := m.View(); !strings.Contains(out, "exited (status 5)") {
		t.Fatalf("a tab whose connection died should say so:\n%s", out)
	}
	deadPane, _ := paneForSlug(t, client, "k3s-01")

	// Entering a dead tab restarts the connection rather than focusing a corpse.
	m = mustOpen(t, m)
	live, ok := paneForSlug(t, client, "k3s-01")
	if !ok {
		t.Fatal("the tab vanished")
	}
	if live.Dead {
		t.Fatal("entering a dead tab should restart it")
	}
	if live.ID == deadPane.ID {
		t.Error("the dead pane should have been replaced, not reused")
	}
	if names := windows(t, client); len(names) != 2 {
		t.Fatalf("windows = %v, want the dead tab replaced rather than duplicated", names)
	}
}

func TestTabsBindReturnsToTheTreeWindow(t *testing.T) {
	_, client := tabModel(t, 34, "sleep 60")

	out, err := exec.Command("tmux", "-L", client.Socket, "list-keys", "-T", "prefix").Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := string(out); !strings.Contains(got, "select-window -t gscrt/workspace:tree") {
		t.Fatalf("prefix t should select the tree window in tabs layout:\n%s", got)
	}
}
