package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// The workspace is one tmux session on gcrt's own socket, holding a window
// named "tree" whose first pane runs the TUI. Every connection is a pane tagged
// with its slug; visible connections are joined into the tree window, hidden
// ones live on as background windows so they keep running.
const (
	WorkspaceSession = "gscrt/workspace"
	TreeWindow       = "tree"

	slugOption  = "@gcrt_slug"
	envMarker   = "GCRT_WORKSPACE"
	workspaceOn = "1"
)

// Layout is how a connection is shown alongside the tree.
type Layout string

const (
	// LayoutTabs gives every connection its own tmux window — a tab, with the
	// tree in a window of its own.
	LayoutTabs Layout = "tabs"
	// LayoutSplit tiles connections as panes beside the tree, so several are
	// visible at once.
	LayoutSplit Layout = "split"
)

// Pane is one gcrt connection living inside the workspace.
type Pane struct {
	ID           string
	Slug         string
	Window       string
	WindowActive bool
	Dead         bool
	Exit         int
}

// Visible reports whether the pane is tiled beside the tree (split layout).
func (p Pane) Visible() bool { return p.Window == TreeWindow }

// Open reports whether the pane is the one on screen, in either layout: in
// tabs that means its window is the current one, in split that it is in the
// tree window.
func (p Pane) Open(layout Layout) bool {
	if layout == LayoutTabs {
		return p.WindowActive
	}
	return p.Visible()
}

func (c *Client) SetLayout(l Layout) { c.Layout = l }

func (c *Client) layout() Layout {
	if c.Layout == "" {
		return LayoutSplit
	}
	return c.Layout
}

func (c *Client) HasTarget(name string) bool {
	cmd := exec.Command(c.Bin, c.args("has-session", "-t", name)...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run() == nil
}

// parsePanes reads list-panes output. It deliberately does not TrimSpace the
// whole blob: a live pane's trailing pane_dead_status is empty, so the last
// line ends in a tab and trimming it would silently drop that pane.
func parsePanes(out string) []Pane {
	var panes []Pane
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) < 6 {
			continue
		}
		p := Pane{ID: f[0], Slug: f[1], Window: f[2], Dead: f[3] == "1", WindowActive: f[5] == "1"}
		p.Exit, _ = strconv.Atoi(f[4])
		panes = append(panes, p)
	}
	return panes
}

func (c *Client) Panes() ([]Pane, error) {
	out, err := c.run("list-panes", "-s", "-t", WorkspaceSession, "-F",
		"#{pane_id}\t#{@gcrt_slug}\t#{window_name}\t#{pane_dead}\t#{pane_dead_status}\t#{window_active}")
	if err != nil {
		if noServer(err.Error()) || strings.Contains(err.Error(), "can't find session") {
			return nil, nil
		}
		return nil, err
	}
	return parsePanes(out), nil
}

// Configure applies the workspace-wide tmux settings. It is idempotent and safe
// to call on every startup, including a reattach.
func (c *Client) Configure(treeWidth int, treePaneID string) error {
	opts := [][2]string{
		{"mouse", "on"},
		{"remain-on-exit", "on"},
		{"automatic-rename", "off"},
		{"history-limit", "50000"},
		{"set-clipboard", "on"},
		{"status", "on"},
	}

	if c.layout() == LayoutTabs {
		// Each connection is a window, so the status bar is the tab bar; a
		// bullet marks a tab with output waiting. A pane border would only
		// steal a row from a window that holds one pane.
		opts = append(opts,
			[2]string{"pane-border-status", "off"},
			[2]string{"monitor-activity", "on"},
			[2]string{"window-status-format", " #W#{?window_activity_flag,•,} "},
			[2]string{"window-status-current-format", "#[reverse,bold] #W #[default]"},
		)
	} else {
		opts = append(opts,
			[2]string{"pane-border-status", "top"},
			[2]string{"pane-border-format", " #{?@gcrt_slug,#{@gcrt_slug},tree} "},
		)
	}

	for _, o := range opts {
		if _, err := c.run("set-option", "-g", o[0], o[1]); err != nil {
			return err
		}
	}

	if c.layout() == LayoutSplit {
		if err := c.Retile(treeWidth); err != nil {
			return err
		}
	} else {
		// The tree redraws whenever we leave it, which would flag it as having
		// activity forever. Only connections should show that marker.
		if _, err := c.run("set-option", "-w", "-t", WorkspaceSession+":"+TreeWindow,
			"monitor-activity", "off"); err != nil {
			return err
		}
	}

	if treePaneID != "" {
		// A quick way back to the tree from a busy session.
		args := []string{"bind-key", "-T", "prefix", "t"}
		if c.layout() == LayoutTabs {
			// select-pane does not cross windows, so target the window.
			args = append(args, "select-window", "-t", WorkspaceSession+":"+TreeWindow)
		} else {
			args = append(args, "select-pane", "-t", treePaneID)
		}
		if _, err := c.run(args...); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) Retile(treeWidth int) error {
	target := WorkspaceSession + ":" + TreeWindow
	if _, err := c.run("set-option", "-w", "-t", target, "main-pane-width", strconv.Itoa(treeWidth)); err != nil {
		return err
	}
	_, err := c.run("select-layout", "-t", target, "main-vertical")
	return err
}

// Show makes a connection the one on screen, creating it first if it does not
// exist yet: a pane tiled beside the tree in split layout, the current tab in
// tabs layout.
func (c *Client) Show(slug string, argv, env []string, treeWidth int) (string, error) {
	if c.layout() == LayoutTabs {
		return c.showTab(slug, argv, env)
	}

	panes, err := c.Panes()
	if err != nil {
		return "", err
	}

	recreate := false
	for _, p := range panes {
		if p.Slug != slug {
			continue
		}
		if p.Dead {
			// A dead pane cannot be revived; drop it and build it again.
			if _, err := c.run("kill-pane", "-t", p.ID); err != nil {
				return "", err
			}
			recreate = true
			break
		}
		if p.Visible() {
			return p.ID, c.Focus(p.ID)
		}
		if _, err := c.run("join-pane", "-d", "-s", p.ID, "-t", WorkspaceSession+":"+TreeWindow); err != nil {
			return "", err
		}
		if err := c.Retile(treeWidth); err != nil {
			return "", err
		}
		return p.ID, c.Focus(p.ID)
	}
	_ = recreate

	id, err := c.newSessionWindow(slug, argv, env)
	if err != nil {
		return "", err
	}
	if _, err := c.run("join-pane", "-d", "-s", id, "-t", WorkspaceSession+":"+TreeWindow); err != nil {
		return "", err
	}
	if err := c.Retile(treeWidth); err != nil {
		return "", err
	}
	return id, c.Focus(id)
}

// showTab selects the connection's window, creating it if needed.
func (c *Client) showTab(slug string, argv, env []string) (string, error) {
	panes, err := c.Panes()
	if err != nil {
		return "", err
	}

	for _, p := range panes {
		if p.Slug != slug {
			continue
		}
		if p.Dead {
			if _, err := c.run("kill-pane", "-t", p.ID); err != nil {
				return "", err
			}
			break
		}
		if _, err := c.run("select-window", "-t", WorkspaceSession+":"+p.Window); err != nil {
			return "", err
		}
		return p.ID, nil
	}

	id, err := c.newSessionWindow(slug, argv, env)
	if err != nil {
		return "", err
	}
	if _, err := c.run("select-window", "-t", WorkspaceSession+":"+slug); err != nil {
		return "", err
	}
	return id, nil
}

func (c *Client) newSessionWindow(slug string, argv, env []string) (string, error) {
	args := []string{"new-window", "-d", "-P", "-F", "#{pane_id}",
		"-t", WorkspaceSession, "-n", slug}
	for _, e := range env {
		args = append(args, "-e", e)
	}
	args = append(args, argv...)

	out, err := c.run(args...)
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(out)
	if id == "" {
		return "", fmt.Errorf("tmux did not report a pane id for %s", slug)
	}
	if _, err := c.run("set-option", "-p", "-t", id, slugOption, slug); err != nil {
		return "", err
	}
	if _, err := c.run("select-pane", "-t", id, "-T", slug); err != nil {
		return "", err
	}
	return id, nil
}

// Hide pulls a pane out of the tree window into its own background window, so
// the process keeps running while it is off screen.
func (c *Client) Hide(paneID, slug string) error {
	_, err := c.run("break-pane", "-d", "-s", paneID, "-n", slug)
	return err
}

func (c *Client) Focus(paneID string) error {
	_, err := c.run("select-pane", "-t", paneID)
	return err
}

func (c *Client) KillPane(paneID string) error {
	_, err := c.run("kill-pane", "-t", paneID)
	return err
}

// DetachSelf drops the attached client, leaving the whole workspace running.
// Detaching when nothing is attached is a no-op, not an error.
func (c *Client) DetachSelf() error {
	if _, err := c.run("detach-client", "-s", WorkspaceSession); err != nil {
		if strings.Contains(err.Error(), "no current client") {
			return nil
		}
		return err
	}
	return nil
}

func (c *Client) ShutdownWorkspace() error {
	_, err := c.run("kill-session", "-t", WorkspaceSession)
	return err
}

// InWorkspace reports whether this process is the tree pane of a gcrt
// workspace, as opposed to a plain TUI running in someone's terminal.
func InWorkspace() bool {
	return os.Getenv(envMarker) == workspaceOn
}
