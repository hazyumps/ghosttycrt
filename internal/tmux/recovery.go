package tmux

import (
	"fmt"
	"strings"
)

// ConfigureTreeControls belongs to tmux, so these controls keep working when
// the TUI has stopped. Never force-respawn a live tree: it may have an unsaved
// session form open.
func (c *Client) ConfigureTreeControls(paneID string) error {
	if paneID == "" {
		return nil
	}
	if err := c.runQuiet("set-option", "-t", WorkspaceSession, "@gcrt_tree_pane", paneID); err != nil {
		return err
	}
	for _, option := range [][2]string{
		{"@gcrt_tree", "1"},
		{"remain-on-exit", "on"},
		{"remain-on-exit-format", "Session manager stopped. Click Sessions below or press F2 to restore it."},
	} {
		if err := c.runQuiet("set-option", "-p", "-t", paneID, option[0], option[1]); err != nil {
			return err
		}
	}
	for _, option := range [][2]string{
		{"status", "on"},
		{"status-left", "#[bold] [ Sessions · F2 ] #[default]"},
		{"status-left-length", "22"},
		{"status-right", " Ctrl-b d: detach "},
	} {
		if err := c.runQuiet("set-option", "-t", WorkspaceSession, option[0], option[1]); err != nil {
			return err
		}
	}
	// A tmux command list, not a shell command. Pane IDs are returned by tmux.
	command := fmt.Sprintf("if-shell -F -t %s '#{pane_dead}' 'respawn-pane -t %s' ; select-window -t %s ; select-pane -t %s",
		paneID, paneID, paneID, paneID)
	for _, binding := range [][2]string{
		{"prefix", "t"}, {"prefix", "R"}, {"root", "F2"}, {"root", "MouseDown1StatusLeft"},
	} {
		if err := c.runQuiet("bind-key", "-T", binding[0], binding[1], command); err != nil {
			return err
		}
	}
	return nil
}

// RecoverTree repairs an existing workspace before attach. It also migrates
// older workspaces whose first tree-window pane predates the @gcrt_tree tag.
// A healthy tree and every connection keep their process and scrollback.
// argv is the current executable plus its path flags; nil reuses the stored
// command. No -k is used, so a concurrent recovery cannot kill a live process.
func (c *Client) RecoverTree(argv []string) error {
	registered, err := c.run("show-options", "-qv", "-t", WorkspaceSession, "@gcrt_tree_pane")
	if err != nil {
		return err
	}
	registered = strings.TrimSpace(registered)
	out, err := c.run("list-panes", "-t", WorkspaceSession+":"+TreeWindow,
		"-F", "#{pane_id}\t#{pane_dead}\t#{@gcrt_tree}")
	if err != nil {
		return err
	}
	var paneID string
	var dead bool
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 3 {
			continue
		}
		if registered != "" && fields[0] != registered {
			continue
		}
		if paneID == "" || fields[2] == "1" {
			paneID, dead = fields[0], fields[1] == "1"
		}
		if fields[2] == "1" {
			break
		}
	}
	if paneID == "" {
		return fmt.Errorf("workspace has no session manager pane")
	}
	c.TreePane = paneID
	if err := c.ConfigureTreeControls(paneID); err != nil {
		return err
	}
	if dead {
		args := append([]string{"respawn-pane", "-t", paneID}, argv...)
		if err := c.runQuiet(args...); err != nil {
			return err
		}
	}
	if err := c.runQuiet("select-window", "-t", paneID); err != nil {
		return err
	}
	return c.Focus(paneID)
}
