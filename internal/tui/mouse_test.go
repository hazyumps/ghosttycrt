package tui_test

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hazyumps/ghosttycrt/internal/session"
	"github.com/hazyumps/ghosttycrt/internal/tui"
)

func click(t *testing.T, m *tui.Model, y int) (*tui.Model, tea.Cmd) {
	t.Helper()
	updated, cmd := m.Update(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      3,
		Y:      y,
	})
	return updated.(*tui.Model), cmd
}

func wheel(t *testing.T, m *tui.Model, button tea.MouseButton) *tui.Model {
	t.Helper()
	updated, _ := m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: button, X: 3, Y: 5})
	return updated.(*tui.Model)
}

// sample rows: 0 network, 1 switches, 2 core-sw-01, 3 servers, 4 k3s-01
func TestMouseClickSelectsARow(t *testing.T) {
	m := newModel(t, sample(), 100, 30)

	m, cmd := click(t, m, 5)
	if cmd != nil {
		t.Fatal("a first click should select, not connect")
	}
	if got := m.SelectedName(); got != "k3s-01" {
		t.Fatalf("selection = %q, want k3s-01", got)
	}
}

func TestMouseClickOnTheSelectedSessionConnects(t *testing.T) {
	m := newModel(t, sample(), 100, 30)

	m, _ = click(t, m, 5)
	m, cmd := click(t, m, 5)
	if cmd == nil {
		t.Fatal("clicking an already-selected session should connect")
	}
}

func TestMouseClickOnAGroupFoldsIt(t *testing.T) {
	m := newModel(t, sample(), 100, 30)

	m, _ = click(t, m, 1) // the "network" group header
	if out := m.View(); strings.Contains(out, "core-sw-01") {
		t.Errorf("clicking a group header should collapse it:\n%s", out)
	}
	m, _ = click(t, m, 1)
	if out := m.View(); !strings.Contains(out, "core-sw-01") {
		t.Errorf("clicking the header again should expand it:\n%s", out)
	}
}

func TestWheelMovesTheCursor(t *testing.T) {
	m := newModel(t, sample(), 100, 30)
	if got := m.SelectedName(); got != "core-sw-01" {
		t.Fatalf("initial selection = %q", got)
	}

	m = wheel(t, m, tea.MouseButtonWheelDown)
	if got := m.SelectedName(); got != "k3s-01" {
		t.Fatalf("after wheel down selection = %q, want k3s-01", got)
	}
	m = wheel(t, m, tea.MouseButtonWheelUp)
	if got := m.SelectedName(); got != "switches" {
		t.Fatalf("after wheel up selection = %q, want switches (three rows up)", got)
	}
}

func manySessions(n int) *session.File {
	f := &session.File{}
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("host-%02d", i)
		f.Session = append(f.Session, session.Session{
			ID: name, Name: name, Slug: name, Transport: session.TransportSSH,
			Group: "fleet",
			SSH:   &session.SSHConfig{Host: "10.0.0.1"},
		})
	}
	return f
}

// A tree taller than the pane must scroll, or the cursor walks off screen.
func TestCursorStaysVisibleInALongTree(t *testing.T) {
	m := newModel(t, manySessions(60), 100, 20)

	m, _ = press(t, m, "G")
	out := m.View()
	if !strings.Contains(out, "host-59") {
		t.Fatalf("the last row should be visible after G:\n%s", out)
	}
	if strings.Contains(out, "host-00") {
		t.Errorf("the first row should have scrolled away:\n%s", out)
	}
	if !strings.Contains(out, "/61") {
		t.Errorf("expected a scroll indicator in the header:\n%s", out)
	}
}

func TestShortTreeHasNoScrollIndicator(t *testing.T) {
	m := newModel(t, manySessions(3), 100, 20)
	if out := m.View(); strings.Contains(out, "1–4/4") {
		t.Errorf("a tree that fits should not show a scroll range:\n%s", out)
	}
}

func TestClickMapsThroughTheScrollOffset(t *testing.T) {
	m := newModel(t, manySessions(60), 100, 20)

	m, _ = press(t, m, "G")
	// After scrolling, screen row 2 is not tree row 1: offset must be applied.
	m, _ = click(t, m, 2)
	if got := m.SelectedName(); got != "host-43" {
		t.Fatalf("selection = %q, want host-43 (offset must be applied)", got)
	}
}
