package tmux_test

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/hazyumps/ghosttycrt/internal/tmux"
)

func tmuxRun(t *testing.T, c *tmux.Client, args ...string) string {
	t.Helper()
	out, err := exec.Command(c.Bin, append([]string{"-L", c.Socket}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("tmux %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func treeState(t *testing.T, c *tmux.Client) string {
	t.Helper()
	return tmuxRun(t, c, "display-message", "-p", "-t", "%0", "#{pane_dead}")
}

func runBinding(t *testing.T, c *tmux.Client, command string) {
	t.Helper()
	cmd := exec.Command(c.Bin, "-L", c.Socket, "source-file", "-")
	cmd.Stdin = strings.NewReader(command + "\n")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("binding failed: %v: %s", err, out)
	}
}

func stopTree(t *testing.T, c *tmux.Client) {
	t.Helper()
	tmuxRun(t, c, "respawn-pane", "-k", "-t", "%0", "sh", "-c", "exit 0")
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if treeState(t, c) == "1" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("test tree did not exit")
}

func TestRecoverTreePreservesConnectionAndHealthyTree(t *testing.T) {
	for _, layout := range []tmux.Layout{tmux.LayoutSplit, tmux.LayoutTabs, tmux.LayoutSidebar} {
		t.Run(string(layout), func(t *testing.T) {
			c := wsClient(t, 30)
			c.TreePane, c.TabsSocket = "%0", c.Socket+"-tabs"
			t.Cleanup(func() { exec.Command("tmux", "-L", c.TabsSocket, "kill-server").Run() })
			c.SetLayout(layout)
			if err := c.Configure(30, "%0"); err != nil {
				t.Fatal(err)
			}
			id, err := c.Show("connection", []string{"sleep", "300"}, nil, 30)
			if err != nil {
				t.Fatal(err)
			}
			connectionClient := c
			if layout == tmux.LayoutSidebar {
				connectionClient = tmux.New(c.TabsSocket)
			}
			pid := tmuxRun(t, connectionClient, "display-message", "-p", "-t", id, "#{pane_pid}")
			stopTree(t, c)
			if err := c.RecoverTree([]string{"sleep", "300"}); err != nil {
				t.Fatal(err)
			}
			if treeState(t, c) != "0" {
				t.Fatal("recovery did not revive the tree")
			}
			if after := tmuxRun(t, connectionClient, "display-message", "-p", "-t", id, "#{pane_pid}"); after != pid {
				t.Fatalf("recovery replaced the connection: %s -> %s", pid, after)
			}
			treePID := tmuxRun(t, c, "display-message", "-p", "-t", "%0", "#{pane_pid}")
			if err := c.RecoverTree([]string{"sh", "-c", "exit 9"}); err != nil {
				t.Fatal(err)
			}
			if after := tmuxRun(t, c, "display-message", "-p", "-t", "%0", "#{pane_pid}"); after != treePID {
				t.Fatal("recovery restarted a healthy tree")
			}
			if focus := tmuxRun(t, c, "display-message", "-p", "-t", "%0", "#{window_active}:#{pane_active}"); focus != "1:1" {
				t.Fatalf("recovery did not focus the tree: %s", focus)
			}
		})
	}
}

func TestRecoverLegacyTreeInstallsControls(t *testing.T) {
	c := wsClient(t, 30)
	// No tag, no recovery binding: this is the state of the reported workspace.
	tmuxRun(t, c, "set-option", "-p", "-t", "%0", "remain-on-exit", "on")
	stopTree(t, c)
	if err := c.RecoverTree([]string{"sleep", "300"}); err != nil {
		t.Fatal(err)
	}
	if treeState(t, c) != "0" {
		t.Fatal("legacy tree stayed dead")
	}
	if status := tmuxRun(t, c, "show-options", "-v", "-t", tmux.WorkspaceSession, "status-left"); !strings.Contains(status, "Sessions") {
		t.Fatal("recovery control is not visible")
	}
}

func TestTreeControlsRestoreDeadTreeAndNeverRestartLiveTree(t *testing.T) {
	for _, key := range []struct{ table, key string }{
		{"root", "F2"}, {"root", "MouseDown1StatusLeft"}, {"prefix", "t"}, {"prefix", "R"},
	} {
		t.Run(key.key, func(t *testing.T) {
			c := wsClient(t, 30)
			if err := c.ConfigureTreeControls("%0"); err != nil {
				t.Fatal(err)
			}
			bindings := tmuxRun(t, c, "list-keys", "-T", key.table)
			binding := ""
			for _, line := range strings.Split(bindings, "\n") {
				fields := strings.Fields(line)
				if len(fields) > 3 && fields[3] == key.key {
					binding = line
					break
				}
			}
			parts := strings.SplitN(binding, key.key+" ", 2)
			if len(parts) != 2 {
				t.Fatalf("missing binding: %q", binding)
			}
			command := strings.ReplaceAll(strings.TrimSpace(parts[1]), `\;`, `;`)
			// Reuse a command that runs after recovery but exits once for the test.
			marker := t.TempDir() + "/started"
			tmuxRun(t, c, "respawn-pane", "-k", "-t", "%0", "sh", "-c", `if [ ! -f "$1" ]; then touch "$1"; exit 0; fi; exec sleep 300`, "sh", marker)
			deadline := time.Now().Add(3 * time.Second)
			for treeState(t, c) != "1" && time.Now().Before(deadline) {
				time.Sleep(10 * time.Millisecond)
			}
			if treeState(t, c) != "1" {
				t.Fatal("tree did not stop")
			}
			// Execute the actual command list saved by bind-key. This catches
			// quoting/parser failures that merely inspecting the binding misses.
			runBinding(t, c, command)
			if treeState(t, c) != "0" {
				t.Fatal("control did not restore the tree")
			}
			pid := tmuxRun(t, c, "display-message", "-p", "-t", "%0", "#{pane_pid}")
			runBinding(t, c, command)
			if after := tmuxRun(t, c, "display-message", "-p", "-t", "%0", "#{pane_pid}"); after != pid {
				t.Fatal("control restarted a live tree")
			}
		})
	}
}

func TestRecoveryDoesNotMistakeContentForRemovedTree(t *testing.T) {
	c := wsClient(t, 30)
	if err := c.ConfigureTreeControls("%0"); err != nil {
		t.Fatal(err)
	}
	tmuxRun(t, c, "split-window", "-h", "-t", "%0", "sleep", "300")
	tmuxRun(t, c, "kill-pane", "-t", "%0")
	if err := c.RecoverTree([]string{"sleep", "300"}); err == nil {
		t.Fatal("recovery treated the content pane as the missing tree")
	}
}
