package tui_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hazyumps/ghosttycrt/internal/tui"
)

func rightClick(t *testing.T, m *tui.Model, x, y int) (*tui.Model, tea.Cmd) {
	t.Helper()
	updated, cmd := m.Update(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonRight,
		X:      x,
		Y:      y,
	})
	return updated.(*tui.Model), cmd
}

// rowOf finds the screen row a piece of text was drawn on, so a test can click
// what it can see rather than guessing at the layout maths.
func rowOf(t *testing.T, m *tui.Model, needle string) int {
	t.Helper()
	for i, line := range strings.Split(m.View(), "\n") {
		if strings.Contains(stripANSI(line), needle) {
			return i
		}
	}
	t.Fatalf("no rendered line contains %q", needle)
	return -1
}

// Tree rows: 0 network, 1 switches, 2 core-sw-01, 3 servers, 4 k3s-01, so the
// screen row of a session is its index plus the header row.
const (
	screenCoreSW01 = 3
	screenServers  = 4
	screenK3s01    = 5
)

func TestRightClickOpensTheSessionMenu(t *testing.T) {
	m, _ := tabModel(t, 34, "sleep 60")
	m = selectK3s(t, m)
	m = mustOpen(t, m)

	m, cmd := rightClick(t, m, 5, screenK3s01)
	if cmd != nil {
		t.Fatal("opening the menu should not run anything yet")
	}
	out := m.View()
	if !strings.Contains(out, "Kill") {
		t.Fatalf("right-click should open the action menu:\n%s", out)
	}
	if strings.Contains(out, "is not running") {
		t.Errorf("the menu did not open for a running session:\n%s", out)
	}
}

func TestRightClickOnAGroupDoesNothing(t *testing.T) {
	m, _ := tabModel(t, 34, "sleep 60")
	m = selectK3s(t, m)
	m = mustOpen(t, m)

	m, _ = rightClick(t, m, 5, screenServers)
	if out := m.View(); strings.Contains(out, "choose an action") {
		t.Fatalf("a group header has no session menu:\n%s", out)
	}
}

// The menu is sticky: a click that misses it must not dismiss it.
func TestMenuStaysUpWhenClickedElsewhere(t *testing.T) {
	m, _ := tabModel(t, 34, "sleep 60")
	m = selectK3s(t, m)
	m = mustOpen(t, m)

	m, _ = rightClick(t, m, 5, screenK3s01)
	if !strings.Contains(m.View(), "choose an action") {
		t.Fatal("the menu did not open")
	}

	m, _ = clickAt(t, m, 2, 12) // somewhere else entirely
	if !strings.Contains(m.View(), "choose an action") {
		t.Fatal("a click outside the menu dismissed it")
	}

	m, _ = rightClick(t, m, 5, screenK3s01)
	if !strings.Contains(m.View(), "choose an action") {
		t.Fatal("a second right-click dismissed the menu")
	}
}

// Clicking a menu item must hit the row that was actually drawn.
func TestClickingAMenuItemRunsIt(t *testing.T) {
	m, _ := tabModel(t, 34, "sleep 60")
	m = selectK3s(t, m)
	m = mustOpen(t, m)

	m, _ = rightClick(t, m, 5, screenK3s01)
	killRow := rowOf(t, m, "Kill")

	m, _ = clickAt(t, m, 8, killRow)
	if out := m.View(); !strings.Contains(out, "Kill k3s-01?") {
		t.Fatalf("clicking Kill should ask for confirmation:\n%s", out)
	}
}

func TestClickingKillInTheMenuClosesTheSession(t *testing.T) {
	m, client := tabModel(t, 34, "sleep 60")
	m = selectK3s(t, m)
	m = mustOpen(t, m)

	m, _ = rightClick(t, m, 5, screenK3s01)
	m, _ = clickAt(t, m, 8, rowOf(t, m, "Kill"))
	if out := m.View(); !strings.Contains(out, "Kill k3s-01?") {
		t.Fatalf("expected a confirmation:\n%s", out)
	}

	m, _ = press(t, m, "y")
	if got := windows(t, client); len(got) != 1 { // just the tree window
		t.Fatalf("windows = %v, want the session closed", got)
	}
}

// A session that exits cleanly should close itself, tab and all.
func TestCleanExitClosesTheSession(t *testing.T) {
	// Wide, so the details column is drawn and the state line is visible.
	m, client := tabModel(t, 120, "exit 0")
	m = selectK3s(t, m)
	m = mustOpen(t, m)

	for i := 0; i < 60; i++ {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
		m = updated.(*tui.Model)
		if len(windows(t, client)) == 1 {
			break
		}
		if i == 59 {
			t.Fatalf("a clean exit left the tab behind: %v", windows(t, client))
		}
	}
	// The window can go between a refresh and this check, so look once more.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = updated.(*tui.Model)

	if out := m.View(); strings.Contains(out, "exited (status") {
		t.Errorf("a clean exit should not be reported as a failure:\n%s", out)
	}
	if out := m.View(); !strings.Contains(out, "not running") {
		t.Errorf("the session should read as simply not running:\n%s", out)
	}
}

// A connection that fails is different: it stays put with its status.
func TestFailedExitKeepsTheSessionVisible(t *testing.T) {
	m, client := tabModel(t, 120, "exit 6")
	m = selectK3s(t, m)
	m = mustOpen(t, m)

	for i := 0; i < 60; i++ {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
		m = updated.(*tui.Model)
		if strings.Contains(m.View(), "exited (status 6)") {
			break
		}
	}
	if !strings.Contains(m.View(), "exited (status 6)") {
		t.Fatalf("a failed connection should stay visible:\n%s", m.View())
	}
	if got := windows(t, client); len(got) != 2 {
		t.Fatalf("windows = %v, want the failed session kept", got)
	}
}

// Nothing tells gcrt when a connection exits, so the tree refreshes on a timer.
func TestInitSchedulesThePeriodicRefresh(t *testing.T) {
	m := newModel(t, sample(), 34, 20)
	if m.Init() == nil {
		t.Fatal("Init should schedule the periodic refresh")
	}
}
