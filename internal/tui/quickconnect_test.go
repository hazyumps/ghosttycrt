package tui_test

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hazyumps/ghosttycrt/internal/tui"
)

func TestQuickConnectSavesAddressThenStartsConnection(t *testing.T) {
	m, path := crudModel(t)
	m, _ = clickAt(t, m, barCol(t, m, "Quick Connect", "Quick"), 0)
	if !strings.Contains(m.View(), "New addresses are saved") {
		t.Fatal("Quick Connect must explain that it saves the address")
	}
	m, _ = press(t, m, "admin@192.0.2.50:2222")
	// Exercise the clickable Connect button, not only the keyboard.
	m, cmd := clickAt(t, m, 8, rowOf(t, m, "[ Connect ]"))
	if cmd == nil {
		t.Fatal("Quick Connect did not start a connection")
	}
	file := reload(t, path)
	s, ok := byName(file, "admin@192.0.2.50:2222")
	if !ok || s.SSH.Host != "192.0.2.50" || s.SSH.Port != 2222 || s.SSH.User != "admin" || s.Group != "Quick Connect" {
		t.Fatalf("saved wrong session: %+v", s)
	}
	if m.SelectedName() != s.Name {
		t.Fatalf("new session not selected: %s", m.SelectedName())
	}
	// Opening the same address again must reuse its record.
	m, _ = press(t, m, "Qadmin@192.0.2.50:2222")
	m, cmd = enterKey(t, m)
	if cmd == nil {
		t.Fatal("second connect did not start")
	}
	if len(reload(t, path).Session) != len(file.Session) {
		t.Fatal("Quick Connect duplicated a saved address")
	}
}

func TestQuickConnectReusesSavedSessionWithoutRewritingItsSettings(t *testing.T) {
	m, path := crudModel(t)
	before, _ := os.ReadFile(path)
	m, _ = press(t, m, "Qpatrick@10.1.3.30")
	m, cmd := enterKey(t, m)
	if cmd == nil {
		t.Fatal("no connect command")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("matching a saved session rewrote the file")
	}
	if m.SelectedName() != "k3s-01" {
		t.Fatalf("selected %s", m.SelectedName())
	}
}

func TestInvalidQuickConnectAndCancelLeaveSessionsAlone(t *testing.T) {
	m, path := crudModel(t)
	before, _ := os.ReadFile(path)
	m, _ = press(t, m, "Qadmin:secret@host")
	m, cmd := enterKey(t, m)
	if cmd != nil {
		t.Fatal("invalid destination launched a connection")
	}
	if !strings.Contains(m.View(), "omit the password") {
		t.Fatal("password error was hidden")
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(*tui.Model)
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("invalid/cancelled Quick Connect wrote a session")
	}
	if strings.Contains(m.View(), "New addresses are saved") {
		t.Fatal("cancel did not close the dialog")
	}
}
