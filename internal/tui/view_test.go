package tui_test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hazyumps/ghosttycrt/internal/config"
	"github.com/hazyumps/ghosttycrt/internal/session"
	"github.com/hazyumps/ghosttycrt/internal/tmux"
	"github.com/hazyumps/ghosttycrt/internal/tui"
)

func sample() *session.File {
	return &session.File{Session: []session.Session{
		{
			ID: "1", Name: "core-sw-01", Slug: "core-sw-01", Transport: session.TransportSSH,
			Group: "network/switches", Pinned: true,
			SSH: &session.SSHConfig{Host: "10.1.3.11", User: "admin"},
			Credentials: &session.CredentialsConfig{
				Provider: "infisical", Ref: "CORE-SW-01-PASS",
			},
		},
		{
			ID: "2", Name: "k3s-01", Slug: "k3s-01", Transport: session.TransportSSH,
			Group: "servers", Tags: []string{"k8s"},
			SSH: &session.SSHConfig{Host: "10.1.3.30", User: "patrick", Jump: "bastion"},
		},
	}}
}

func unitSocket() string { return fmt.Sprintf("gcrt-unit-%d", os.Getpid()) }

func newModel(t *testing.T, file *session.File, w, h int) *tui.Model {
	t.Helper()
	m := tui.New(config.Default(), file, config.DefaultPaths(), session.Problems{}, tmux.New(unitSocket()))
	updated, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return updated.(*tui.Model)
}

func press(t *testing.T, m *tui.Model, keys string) (*tui.Model, tea.Cmd) {
	t.Helper()
	var cmd tea.Cmd
	for _, r := range keys {
		var updated tea.Model
		updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(*tui.Model)
	}
	return m, cmd
}

func pressKey(t *testing.T, m *tui.Model, k tea.KeyType) *tui.Model {
	t.Helper()
	updated, _ := m.Update(tea.KeyMsg{Type: k})
	return updated.(*tui.Model)
}

func TestViewRendersTreeAndDetails(t *testing.T) {
	m := newModel(t, sample(), 100, 30)
	out := m.View()

	for _, want := range []string{"network", "switches", "core-sw-01", "servers", "k3s-01"} {
		if !strings.Contains(out, want) {
			t.Errorf("view is missing %q", want)
		}
	}
	for _, want := range []string{"transport", "credential", "infisical:CORE-SW-01-PASS", "state"} {
		if !strings.Contains(out, want) {
			t.Errorf("details pane is missing %q", want)
		}
	}
}

func TestViewNarrowCollapsesToTree(t *testing.T) {
	m := newModel(t, sample(), 50, 20)
	out := m.View()
	if strings.Contains(out, "credential") {
		t.Error("narrow view should not render the details pane")
	}
	if !strings.Contains(out, "core-sw-01") {
		t.Error("narrow view should still render the tree")
	}
}

func TestFilterNarrowsTheTree(t *testing.T) {
	m := newModel(t, sample(), 100, 30)
	m, _ = press(t, m, "/k3s")

	out := m.View()
	if !strings.Contains(out, "k3s-01") {
		t.Error("filtered view should keep k3s-01")
	}
	if strings.Contains(out, "core-sw-01") {
		t.Error("filtered view should drop core-sw-01")
	}
}

func TestEmptyState(t *testing.T) {
	m := newModel(t, &session.File{}, 100, 30)
	if out := m.View(); !strings.Contains(out, "No sessions yet") {
		t.Errorf("empty state not shown:\n%s", out)
	}
}

func TestCursorStartsOnASession(t *testing.T) {
	m := newModel(t, sample(), 100, 30)
	if got := m.SelectedName(); got != "core-sw-01" {
		t.Fatalf("initial selection = %q, want core-sw-01", got)
	}
}

// A group row is a header, not a target: d must not offer to kill it.
func TestMenuIsNotOfferedForAGroupRow(t *testing.T) {
	m := newModel(t, sample(), 100, 30)
	m = pressKey(t, m, tea.KeyUp) // onto the "network" group row

	m, _ = press(t, m, "d")
	if out := m.View(); strings.Contains(out, "choose an action") {
		t.Errorf("menu opened for a group row:\n%s", out)
	}
}

func TestMenuExplainsWhenNothingIsRunning(t *testing.T) {
	m := newModel(t, sample(), 100, 30)
	m, _ = press(t, m, "d")

	out := m.View()
	if strings.Contains(out, "choose an action") {
		t.Error("menu should not open for a session that is not running")
	}
	if !strings.Contains(out, "not running") {
		t.Errorf("expected an explanation in the footer:\n%s", out)
	}
}

func TestTransportNotImplementedIsVisible(t *testing.T) {
	file := &session.File{Session: []session.Session{{
		ID: "1", Name: "console", Slug: "console", Transport: session.TransportSerial,
		Serial: &session.SerialConfig{Device: "/dev/tty.usbserial-1420", Baud: 9600},
	}}}
	m := newModel(t, file, 100, 30)
	out := m.View()
	if !strings.Contains(out, "M2") {
		t.Errorf("serial session should say when it lands:\n%s", out)
	}
}
