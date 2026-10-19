package tmux_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hazyumps/ghosttycrt/internal/config"
	"github.com/hazyumps/ghosttycrt/internal/session"
	"github.com/hazyumps/ghosttycrt/internal/tmux"
	"github.com/hazyumps/ghosttycrt/internal/transport"
)

// Exercise the environment received by the SSH process after tmux launches it,
// including the sidebar's first-window respawn and later reconnects.
func TestSSHTerminalReachesProcessAcrossLayouts(t *testing.T) {
	for _, layout := range []tmux.Layout{"inline", tmux.LayoutTabs, tmux.LayoutSidebar, tmux.LayoutSplit} {
		t.Run(string(layout), func(t *testing.T) {
			dir := t.TempDir()
			fake := filepath.Join(dir, "ssh")
			if err := os.WriteFile(fake, []byte("#!/bin/sh\nprintf '%s' \"$TERM\" > \"$1\"\nexec sleep 300\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			var c *tmux.Client
			if layout == "inline" {
				c = testClient(t)
			} else {
				c = wsClient(t, 30)
				c.TreePane, c.TabsSocket = "%0", c.Socket+"-tabs"
				t.Cleanup(func() { exec.Command("tmux", "-L", c.TabsSocket, "kill-server").Run() })
				c.SetLayout(layout)
				if err := c.Configure(30, "%0"); err != nil {
					t.Fatal(err)
				}
			}
			cfg := config.Default()
			cfg.Transports.SSH.Binary = fake
			launch := func(slug, term, marker string, reconnect bool) {
				t.Helper()
				want := term
				if want == "" {
					host := c
					if layout == tmux.LayoutSidebar {
						host = tmux.New(c.TabsSocket)
					}
					want = strings.TrimSpace(tmuxRun(t, host, "show-options", "-gv", "default-terminal"))
				}
				s := &session.Session{SSH: &session.SSHConfig{Host: marker, Terminal: term}, Transport: session.TransportSSH}
				tr, err := transport.For(s, cfg)
				if err != nil {
					t.Fatal(err)
				}
				argv, env, err := tr.Argv(s, "")
				if err != nil {
					t.Fatal(err)
				}
				if layout == "inline" {
					if reconnect {
						err = c.ReconnectSession(slug, argv, env, nil, true)
					} else {
						err = c.Ensure(slug, argv, env, nil)
					}
				} else {
					if reconnect {
						_, err = c.Reconnect(slug, argv, env, 30, true)
					} else {
						_, err = c.Show(slug, argv, env, 30)
					}
				}
				if err != nil {
					t.Fatal(err)
				}
				deadline := time.Now().Add(3 * time.Second)
				for time.Now().Before(deadline) {
					if b, err := os.ReadFile(marker); err == nil && len(b) > 0 {
						if string(b) != want {
							t.Fatalf("SSH received TERM=%q, want %q", b, want)
						}
						return
					}
					time.Sleep(20 * time.Millisecond)
				}
				t.Fatal("SSH stand-in never reported TERM")
			}
			launch("first", "xterm", filepath.Join(dir, "first"), false)
			host := c
			if layout == tmux.LayoutSidebar {
				host = tmux.New(c.TabsSocket)
			}
			serverTerm := tmuxRun(t, host, "show-options", "-gv", "default-terminal")
			launch("other", "", filepath.Join(dir, "other"), false)
			before := tmuxRun(t, host, "list-panes", "-a", "-F", "#{pane_pid}")
			launch("first", "vt100", filepath.Join(dir, "reconnected"), true)
			if tmuxRun(t, host, "show-options", "-gv", "default-terminal") != serverTerm {
				t.Fatal("terminal override changed the server default")
			}
			after := tmuxRun(t, host, "list-panes", "-a", "-F", "#{pane_pid}")
			unchanged := 0
			for _, pid := range strings.Fields(before) {
				for _, next := range strings.Fields(after) {
					if pid == next {
						unchanged++
					}
				}
			}
			if unchanged != len(strings.Fields(before))-1 {
				t.Fatal("reconnect did not preserve all other processes")
			}
			launch("first", "", filepath.Join(dir, "inherited-again"), true)
		})
	}
}
