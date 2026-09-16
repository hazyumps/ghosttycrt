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

// Pane is one gcrt connection living inside the workspace.
type Pane struct {
	ID     string
	Slug   string
	Window string
	Dead   bool
	Exit   int
}

// Visible reports whether the pane is currently tiled beside the tree.
func (p Pane) Visible() bool { return p.Window == TreeWindow }

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
		if len(f) < 5 {
			continue
		}
		p := Pane{ID: f[0], Slug: f[1], Window: f[2], Dead: f[3] == "1"}
		p.Exit, _ = strconv.Atoi(f[4])
		panes = append(panes, p)
	}
	return panes
}

func (c *Client) Panes() ([]Pane, error) {
	out, err := c.run("list-panes", "-s", "-t", WorkspaceSession, "-F",
		"#{pane_id}\t#{@gcrt_slug}\t#{window_name}\t#{pane_dead}\t#{pane_dead_status}")
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
		{"pane-border-status", "top"},
		{"pane-border-format", " #{?@gcrt_slug,#{@gcrt_slug},tree} "},
		{"history-limit", "50000"},
		{"set-clipboard", "on"},
	}
	for _, o := range opts {
		if _, err := c.run("set-option", "-g", o[0], o[1]); err != nil {
			return err
		}
	}
	if err := c.Retile(treeWidth); err != nil {
		return err
	}
	if treePaneID != "" {
		// A quick way back to the tree from a busy session pane.
		if _, err := c.run("bind-key", "-T", "prefix", "t", "select-pane", "-t", treePaneID); err != nil {
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

// Show makes a connection visible beside the tree and focuses it, creating it
// first if it does not exist yet.
func (c *Client) Show(slug string, argv, env []string, treeWidth int) (string, error) {
	panes, err := c.Panes()
	if err != nil {
		return "", err
	}

	for _, p := range panes {
		if p.Slug != slug {
			continue
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
