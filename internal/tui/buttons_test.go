package tui_test

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hazyumps/ghosttycrt/internal/tui"
)

func clickText(t *testing.T, m *tui.Model, label string) (*tui.Model, tea.Cmd) {
	t.Helper()
	for y, line := range strings.Split(m.View(), "\n") {
		line = stripANSI(line)
		if x := strings.Index(line, label); x >= 0 {
			return clickAt(t, m, len([]rune(line[:x]))+1, y)
		}
	}
	t.Fatalf("missing clickable text %q:\n%s", label, m.View())
	return m, nil
}

func TestConfirmButtonsRequireAnActualHitAndNoDoesNotWrite(t *testing.T) {
	for _, width := range []int{34, 80, 120} {
		m, path := crudModel(t)
		before, _ := os.ReadFile(path)
		m, _ = press(t, m, "d")
		m, _ = clickText(t, m, "Forget")
		updated, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 20})
		m = updated.(*tui.Model)
		m, _ = clickAt(t, m, 0, rowOf(t, m, "[ Yes ]"))
		if !strings.Contains(m.View(), "Confirm") {
			t.Fatal("blank space activated a confirmation")
		}
		m, _ = clickText(t, m, "[ No ]")
		after, _ := os.ReadFile(path)
		if strings.Contains(m.View(), "Confirm") || string(before) != string(after) {
			t.Fatal("No must dismiss without changing the session file")
		}
		m, _ = press(t, m, "d")
		m, _ = clickText(t, m, "Forget")
		m, _ = clickText(t, m, "[ Yes ]")
		if len(reload(t, path).Session) != 1 {
			t.Fatal("Yes did not remove exactly the selected record")
		}
	}
}

func TestMouseEditorSaveCommitsPendingTextAndCancelDoesNot(t *testing.T) {
	m, path := crudModel(t)
	m, _ = press(t, m, "d")
	m, _ = clickText(t, m, "Edit")
	m, _ = clickText(t, m, "name")
	m = pressKey(t, m, tea.KeyCtrlU)
	m, _ = press(t, m, "renamed-by-mouse")
	m, _ = clickText(t, m, "[ Save ]")
	if _, found := byName(reload(t, path), "renamed-by-mouse"); !found {
		t.Fatal("mouse Save lost the text currently being edited")
	}
	before, _ := os.ReadFile(path)
	m, _ = clickAt(t, m, barCol(t, m, "New", "+"), 0)
	m, _ = press(t, m, "discard-me")
	m, _ = clickText(t, m, "[ Cancel ]")
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("Cancel wrote an unfinished form")
	}
}

func TestMouseCanLeaveQuickConnectHelpAndMenus(t *testing.T) {
	m, path := crudModel(t)
	before, _ := os.ReadFile(path)
	m, _ = clickAt(t, m, barCol(t, m, "Quick Connect", "Quick"), 0)
	m, _ = press(t, m, "unused.example")
	m, _ = clickText(t, m, "[ Cancel ]")
	m, _ = clickAt(t, m, barCol(t, m, "Help", "?"), 0)
	m, _ = clickText(t, m, "[ Close ]")
	m, _ = rightClick(t, m, 8, screenCoreSW01)
	m, _ = clickText(t, m, "[ Cancel ]")
	after, _ := os.ReadFile(path)
	if string(before) != string(after) || strings.Contains(m.View(), "choose an action") {
		t.Fatal("mouse cancel/close did not return to the unchanged tree")
	}
}

func TestMouseCanClearFilterAndPinOrDuplicateSession(t *testing.T) {
	m, path := crudModel(t)
	m, _ = clickAt(t, m, barCol(t, m, "Filter", "/"), 0)
	m, _ = press(t, m, "k3s")
	m, _ = clickAt(t, m, barCol(t, m, "Done", "Done"), 0)
	m, _ = clickAt(t, m, barCol(t, m, "Clear", "Clear"), 0)
	m, _ = rightClick(t, m, 8, screenK3s01)
	m, _ = clickText(t, m, "Pin")
	s, _ := byName(reload(t, path), "k3s-01")
	if !s.Pinned {
		t.Fatal("mouse Pin did not persist")
	}
	m, _ = press(t, m, "d")
	m, _ = clickText(t, m, "Duplicate")
	m, _ = clickText(t, m, "[ Save ]")
	if _, found := byName(reload(t, path), "k3s-01 copy"); !found {
		t.Fatal("mouse Duplicate did not create a copy")
	}
}

func TestShortEditorScrollsToAllFieldsAndKeepsButtonsVisible(t *testing.T) {
	m, _ := crudModel(t)
	m, _ = press(t, m, "e")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 12})
	m = updated.(*tui.Model)
	for i := 0; i < 30; i++ {
		updated, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
		m = updated.(*tui.Model)
		out := stripANSI(m.View())
		if len(strings.Split(out, "\n")) > 12 || !strings.Contains(out, "[ Save ]") || !strings.Contains(out, "[ Cancel ]") {
			t.Fatalf("editor controls overflowed:\n%s", out)
		}
		if strings.Contains(out, "ref") {
			return
		}
	}
	t.Fatal("wheel could not reach the last editor field")
}
