package tui_test

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hazyumps/ghosttycrt/internal/config"
	"github.com/hazyumps/ghosttycrt/internal/session"
	"github.com/hazyumps/ghosttycrt/internal/tmux"
	"github.com/hazyumps/ghosttycrt/internal/tui"
)

// liveModel puts a real tmux session on a private socket and points the model
// at it, so attach/detach/quit behaviour is exercised against real tmux without
// touching the user's sessions.
func liveModel(t *testing.T) (*tui.Model, *tmux.Client) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	socket := fmt.Sprintf("gcrt-tui-%d", os.Getpid())
	client := tmux.New(socket)
	t.Cleanup(func() { exec.Command("tmux", "-L", socket, "kill-server").Run() })

	if err := client.Ensure("k3s-01", []string{"sleep", "60"}, nil, nil); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	m := tui.New(config.Default(), sample(), config.DefaultPaths(), session.Problems{}, client)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return updated.(*tui.Model), client
}

func selectK3s(t *testing.T, m *tui.Model) *tui.Model {
	t.Helper()
	for i := 0; i < 10 && m.SelectedName() != "k3s-01"; i++ {
		m, _ = press(t, m, "j")
	}
	if m.SelectedName() != "k3s-01" {
		t.Fatalf("could not select k3s-01, stuck on %q", m.SelectedName())
	}
	return m
}

// AC-9 / AC-10, as far as the tree is concerned: the session is running
// detached, and gcrt reports it as reattachable.
func TestRunningSessionShowsAsDetachedAndRunning(t *testing.T) {
	m, _ := liveModel(t)
	m = selectK3s(t, m)

	out := m.View()
	if !strings.Contains(out, "○") {
		t.Errorf("expected the running glyph:\n%s", out)
	}
	if !strings.Contains(out, "running, detached") {
		t.Errorf("details should report a running detached session:\n%s", out)
	}
}

// AC-35: quitting with sessions alive confirms, and the default is the safe
// answer.
func TestQuitConfirmsWhileASessionIsRunning(t *testing.T) {
	m, _ := liveModel(t)

	m, cmd := press(t, m, "q")
	if cmd != nil {
		t.Fatal("q should ask before quitting, not quit")
	}
	if out := m.View(); !strings.Contains(out, "still running") {
		t.Fatalf("expected a confirmation:\n%s", out)
	}

	// n is the default and must not quit.
	m = pressKey(t, m, tea.KeyEnter)
	if out := m.View(); strings.Contains(out, "Confirm") {
		t.Fatalf("confirmation should have been dismissed:\n%s", out)
	}

	m, cmd = press(t, m, "q")
	m, cmd = press(t, m, "y")
	if cmd == nil {
		t.Fatal("y should return the quit command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("y should return tea.Quit")
	}
}

// AC-35: d opens a menu whose default is the safe action, and Kill asks again.
func TestKillMenuConfirmsAndKills(t *testing.T) {
	m, client := liveModel(t)
	m = selectK3s(t, m)

	m, _ = press(t, m, "d")
	out := m.View()
	if !strings.Contains(out, "Detach") || !strings.Contains(out, "Kill") {
		t.Fatalf("menu is missing actions:\n%s", out)
	}
	if !strings.Contains(stripANSI(out), "▸ Detach") {
		t.Errorf("Detach should be the default selection:\n%s", out)
	}

	m, _ = press(t, m, "j")
	m = pressKey(t, m, tea.KeyEnter)
	if out := m.View(); !strings.Contains(out, "Kill k3s-01?") {
		t.Fatalf("expected a kill confirmation:\n%s", out)
	}

	m, _ = press(t, m, "y")
	if client.Has("k3s-01") {
		t.Fatal("session survived the confirmed kill")
	}
	if out := m.View(); !strings.Contains(out, "killed") {
		t.Fatalf("expected a kill status:\n%s", out)
	}
}
