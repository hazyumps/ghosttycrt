package tui_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hazyumps/ghosttycrt/internal/config"
	"github.com/hazyumps/ghosttycrt/internal/session"
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

func sized(t *testing.T, file *session.File, w, h int) tui.Model {
	t.Helper()
	m := tui.New(config.Default(), file, config.DefaultPaths(), session.Problems{})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return updated.(tui.Model)
}

func TestViewRendersTreeAndDetails(t *testing.T) {
	m := sized(t, sample(), 100, 30)
	out := m.View()

	for _, want := range []string{"network", "switches", "core-sw-01", "servers", "k3s-01"} {
		if !strings.Contains(out, want) {
			t.Errorf("view is missing %q", want)
		}
	}
	for _, want := range []string{"transport", "credential", "infisical:CORE-SW-01-PASS"} {
		if !strings.Contains(out, want) {
			t.Errorf("details pane is missing %q", want)
		}
	}
}

func TestViewNarrowCollapsesToTree(t *testing.T) {
	m := sized(t, sample(), 50, 20)
	out := m.View()
	if strings.Contains(out, "credential") {
		t.Error("narrow view should not render the details pane")
	}
	if !strings.Contains(out, "core-sw-01") {
		t.Error("narrow view should still render the tree")
	}
}

func TestFilterNarrowsTheTree(t *testing.T) {
	m := sized(t, sample(), 100, 30)
	m = key(t, m, "/")
	for _, r := range "k3s" {
		m = key(t, m, string(r))
	}
	out := m.View()
	if !strings.Contains(out, "k3s-01") {
		t.Error("filtered view should keep k3s-01")
	}
	if strings.Contains(out, "core-sw-01") {
		t.Error("filtered view should drop core-sw-01")
	}
}

func TestEmptyState(t *testing.T) {
	m := sized(t, &session.File{}, 100, 30)
	out := m.View()
	if !strings.Contains(out, "No sessions yet") {
		t.Errorf("empty state not shown:\n%s", out)
	}
}

func key(t *testing.T, m tui.Model, s string) tui.Model {
	t.Helper()
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	if s == "/" {
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}
	}
	updated, _ := m.Update(msg)
	return updated.(tui.Model)
}
