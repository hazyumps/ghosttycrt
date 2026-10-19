package tmux_test

import (
	"os/exec"
	"testing"

	"github.com/hazyumps/ghosttycrt/internal/tmux"
)

func TestContentUpgradePreservesConnectionsAndOnlyRestartsOnce(t *testing.T) {
	c := wsClient(t, 30)
	c.TreePane, c.TabsSocket = "%0", c.Socket+"-tabs"
	t.Cleanup(func() { exec.Command("tmux", "-L", c.TabsSocket, "kill-server").Run() })
	c.SetLayout(tmux.LayoutSidebar)
	if err := c.Configure(30, "%0"); err != nil {
		t.Fatal(err)
	}
	id, err := c.Show("keep", []string{"sleep", "300"}, nil, 30)
	if err != nil {
		t.Fatal(err)
	}
	tabs := tmux.New(c.TabsSocket)
	pid := tmuxRun(t, tabs, "display-message", "-p", "-t", id, "#{pane_pid}")
	contentPID := tmuxRun(t, c, "display-message", "-p", "-t", "%1", "#{pane_pid}")
	tmuxRun(t, c, "set-option", "-pu", "-t", "%1", "@gcrt_content_hash")
	if err := c.Configure(30, "%0"); err != nil {
		t.Fatal(err)
	}
	upgradedPID := tmuxRun(t, c, "display-message", "-p", "-t", "%1", "#{pane_pid}")
	if upgradedPID == contentPID {
		t.Fatal("legacy display wrapper was not upgraded")
	}
	if err := c.Configure(30, "%0"); err != nil {
		t.Fatal(err)
	}
	if tmuxRun(t, c, "display-message", "-p", "-t", "%1", "#{pane_pid}") != upgradedPID {
		t.Fatal("unchanged display wrapper was needlessly restarted")
	}
	if tmuxRun(t, tabs, "display-message", "-p", "-t", id, "#{pane_pid}") != pid {
		t.Fatal("display upgrade interrupted a connection")
	}
}

func TestReconnectPreservesPaneAndOtherConnections(t *testing.T) {
	for _, layout := range []tmux.Layout{tmux.LayoutTabs, tmux.LayoutSidebar, tmux.LayoutSplit} {
		t.Run(string(layout), func(t *testing.T) {
			c := wsClient(t, 30)
			c.TreePane, c.TabsSocket = "%0", c.Socket+"-tabs"
			t.Cleanup(func() { exec.Command("tmux", "-L", c.TabsSocket, "kill-server").Run() })
			c.SetLayout(layout)
			if err := c.Configure(30, "%0"); err != nil {
				t.Fatal(err)
			}
			id, err := c.Show("first", []string{"sleep", "300"}, nil, 30)
			if err != nil {
				t.Fatal(err)
			}
			other, err := c.Show("other", []string{"sleep", "300"}, nil, 30)
			if err != nil {
				t.Fatal(err)
			}
			host := c
			if layout == tmux.LayoutSidebar {
				host = tmux.New(c.TabsSocket)
			}
			pid := tmuxRun(t, host, "display-message", "-p", "-t", id, "#{pane_pid}")
			otherPID := tmuxRun(t, host, "display-message", "-p", "-t", other, "#{pane_pid}")
			if _, err := c.Reconnect("first", []string{"sleep", "300"}, nil, 30, false); err == nil {
				t.Fatal("unconfirmed reconnect stopped a live connection")
			}
			if layout == tmux.LayoutSplit {
				if err := c.Hide(id, "first"); err != nil {
					t.Fatal(err)
				}
			}
			got, err := c.Reconnect("first", []string{"sleep", "301"}, []string{"GCRT_TEST=reconnected"}, 30, true)
			if err != nil {
				t.Fatal(err)
			}
			if got != id {
				t.Fatal("reconnect replaced the pane")
			}
			if after := tmuxRun(t, host, "display-message", "-p", "-t", id, "#{pane_pid}"); after == pid {
				t.Fatal("reconnect left the old process running")
			}
			if after := tmuxRun(t, host, "display-message", "-p", "-t", other, "#{pane_pid}"); after != otherPID {
				t.Fatal("reconnect interrupted another connection")
			}
			if p, ok := paneBySlug(t, c, "first"); !ok || !p.Open(layout) {
				t.Fatal("reconnect did not show the connection")
			}
		})
	}
}
