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

	// The sidebar layout runs a second server: connections are windows there,
	// and its status line is the tab bar drawn at the top of the content pane.
	TabsSocket  = "gcrt-tabs"
	TabsSession = "gscrt/tabs"

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
	// LayoutSidebar pins the tree on the left and shows connections as tabs in
	// the region beside it, one at a time, via a second tmux server.
	LayoutSidebar Layout = "sidebar"
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

// Open reports whether the pane is the one on screen. In tabs and sidebar that
// means its window is the current one; in split, that it is tiled in the tree
// window.
func (p Pane) Open(layout Layout) bool {
	if layout == LayoutSplit {
		return p.Visible()
	}
	return p.WindowActive
}

func (c *Client) SetLayout(l Layout) { c.Layout = l }

func (c *Client) layout() Layout {
	if c.Layout == "" {
		return LayoutSplit
	}
	return c.Layout
}

// tabClient speaks to the sidebar's second server. Socket is settable so tests
// can keep their servers apart.
func (c *Client) tabClient() *Client {
	socket := c.TabsSocket
	if socket == "" {
		socket = TabsSocket
	}
	return &Client{Socket: socket, Bin: c.Bin}
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

func (c *Client) listPanes(session string) ([]Pane, error) {
	out, err := c.run("list-panes", "-s", "-t", session, "-F",
		"#{pane_id}\t#{@gcrt_slug}\t#{window_name}\t#{pane_dead}\t#{pane_dead_status}\t#{window_active}")
	if err != nil {
		if noServer(err.Error()) || strings.Contains(err.Error(), "can't find session") {
			return nil, nil
		}
		return nil, err
	}
	return parsePanes(out), nil
}

// Panes lists the connections. In the sidebar layout they live in the second
// server, so that is where the tree has to look.
func (c *Client) Panes() ([]Pane, error) {
	if c.layout() == LayoutSidebar {
		return c.tabClient().listPanes(TabsSession)
	}
	return c.listPanes(WorkspaceSession)
}

// Configure applies the workspace-wide tmux settings. It is idempotent and safe
// to call on every startup, including a reattach.
func (c *Client) Configure(treeWidth int, treePaneID string) error {
	opts := [][2]string{
		{"mouse", "on"},
		// "failed" not "on": exiting a session cleanly should close it, while a
		// connection that dies leaves its pane up with the exit status.
		{"remain-on-exit", "failed"},
		{"automatic-rename", "off"},
		{"history-limit", "50000"},
		{"set-clipboard", "on"},
		{"status", "on"},
	}

	if c.layout() == LayoutSplit {
		// Tiled panes need labels to tell them apart.
		opts = append(opts,
			[2]string{"pane-border-status", "top"},
			[2]string{"pane-border-format", " #{?@gcrt_slug,#{@gcrt_slug},tree} "},
		)
	} else {
		// A border would only steal a row: tabs carry their own names in the
		// status line, and the sidebar draws a tab bar inside the content pane.
		opts = append(opts, [2]string{"pane-border-status", "off"})
	}

	if c.layout() == LayoutTabs {
		// Each connection is a window, so the status bar is the tab bar; a
		// bullet marks a tab with output waiting.
		opts = append(opts,
			[2]string{"monitor-activity", "on"},
			[2]string{"window-status-format", " #W#{?window_activity_flag,•,} "},
			[2]string{"window-status-current-format", "#[reverse,bold] #W #[default]"},
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

	if c.layout() == LayoutSidebar {
		if err := c.configureSidebar(treeWidth); err != nil {
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
	switch c.layout() {
	case LayoutTabs:
		return c.showTab(slug, argv, env)
	case LayoutSidebar:
		return c.showSidebar(slug, argv, env)
	}

	panes, err := c.Panes()
	if err != nil {
		return "", err
	}

	for _, p := range panes {
		if p.Slug != slug {
			continue
		}
		if p.Dead {
			// A dead pane cannot be revived; drop it and build it again.
			if _, err := c.run("kill-pane", "-t", p.ID); err != nil {
				return "", err
			}
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
	if c.layout() == LayoutSidebar {
		// The pane lives in the tab server; killing its window is the same act.
		_, err := c.tabClient().run("kill-window", "-t", paneID)
		return err
	}
	_, err := c.run("kill-pane", "-t", paneID)
	return err
}

// TreeVersion is the version of the gcrt that started this workspace, recorded
// on the tree pane. Empty when there is no marker.
func (c *Client) TreeVersion() string {
	if c.TreePane == "" {
		return ""
	}
	out, err := c.run("show-options", "-p", "-t", c.TreePane, "@gcrt_version")
	if err != nil {
		return ""
	}
	fields := strings.Fields(out)
	if len(fields) < 2 {
		return ""
	}
	return fields[len(fields)-1]
}

func (c *Client) SetTreeVersion(v string) error {
	if c.TreePane == "" {
		return nil
	}
	return c.runQuiet("set-option", "-p", "-t", c.TreePane, "@gcrt_version", v)
}

// ToggleTreeZoom makes the tree pane fill the window, and back again. Used for
// views that need more width than a sidebar gives them — the session form. tmux
// resizes the pane, so the TUI is told about the new size and redraws.
func (c *Client) ToggleTreeZoom() error {
	if c.TreePane == "" {
		return nil
	}
	return c.runQuiet("resize-pane", "-Z", "-t", c.TreePane)
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

// ShutdownWorkspace tears down both servers in the sidebar layout.
func (c *Client) ShutdownWorkspace() error {
	if c.layout() == LayoutSidebar {
		_ = c.tabClient().runQuiet("kill-server")
	}
	_, err := c.run("kill-session", "-t", WorkspaceSession)
	return err
}

// InWorkspace reports whether this process is the tree pane of a gcrt
// workspace, as opposed to a plain TUI running in someone's terminal.
func InWorkspace() bool {
	return os.Getenv(envMarker) == workspaceOn
}
