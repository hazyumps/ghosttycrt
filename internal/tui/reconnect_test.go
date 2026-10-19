package tui_test

import (
	"os/exec"
	"strings"
	"testing"
)

func TestReconnectRequiresConfirmationForLiveConnection(t *testing.T) {
	m, c := workspaceModel(t, 100, "sleep 60")
	m = selectK3s(t, m)
	m = mustOpen(t, m)
	p, _ := paneForSlug(t, c, "k3s-01")
	pid := func() string {
		out, err := exec.Command("tmux", "-L", c.Socket, "display-message", "-p", "-t", p.ID, "#{pane_pid}").Output()
		if err != nil {
			t.Fatal(err)
		}
		return string(out)
	}
	before := pid()
	m, cmd := press(t, m, "R")
	if cmd != nil || !strings.Contains(m.View(), "Reconnect k3s-01?") {
		t.Fatal("live reconnect did not ask first")
	}
	m, _ = press(t, m, "n")
	if pid() != before {
		t.Fatal("cancelling restarted the connection")
	}
	m, _ = press(t, m, "d")
	m, _ = clickAt(t, m, 8, rowOf(t, m, "Reconnect"))
	m, cmd = press(t, m, "y")
	if cmd == nil {
		t.Fatal("confirmed reconnect did not start")
	}
	m.Update(cmd())
	if pid() == before {
		t.Fatal("confirmed reconnect did not restart")
	}
}

func TestReconnectOfflineSessionConnectsImmediately(t *testing.T) {
	m, c := workspaceModel(t, 100, "sleep 60")
	m = selectK3s(t, m)
	_, cmd := press(t, m, "R")
	if cmd == nil {
		t.Fatal("offline reconnect should connect directly")
	}
	m.Update(cmd())
	if _, ok := paneForSlug(t, c, "k3s-01"); !ok {
		t.Fatal("no connection after reconnect")
	}
}
