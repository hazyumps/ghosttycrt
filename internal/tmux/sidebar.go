package tmux

import (
	"fmt"
	"strings"
)

// The sidebar layout needs a second tmux server. In tmux a tab *is* a window,
// and a window's status line spans the whole terminal, so there is no way to
// draw a tab bar inside one region of a single window. Instead the content pane
// runs its own server, whose status line — positioned at the top — becomes the
// tab bar for that pane, while the outer session keeps the tree pinned on the
// left.
//
// The usual nested-tmux complaint does not apply: the outer client consumes the
// prefix key long before a pane sees it, so the inner server is driven entirely
// by clicks on its tab bar and by gcrt's own commands.

// contentScript is what the content pane runs: attach to the tab server's
// session when it exists, and say so plainly when it does not. It retries
// because gcrt creates that session only once a connection is opened, and
// because killing the last connection takes the session with it.
func (c *Client) contentScript() string {
	return fmt.Sprintf(`while :; do
  if %[1]s -L %[2]s has-session -t %[3]s 2>/dev/null; then
    %[1]s -L %[2]s attach -t %[3]s
  else
    printf '\r\n  no connections yet — pick one in the tree\r\n'
    sleep 1
  fi
done`, c.Bin, c.tabSocket(), TabsSession)
}

func (c *Client) tabSocket() string {
	if c.TabsSocket != "" {
		return c.TabsSocket
	}
	return TabsSocket
}

// configureSidebar prepares the content pane and the second server that draws
// the tab bar inside it. It is idempotent, so a reattach reuses both.
func (c *Client) configureSidebar(treeWidth int) error {
	if err := c.ensureContentPane(); err != nil {
		return err
	}
	if err := c.configureTabServer(); err != nil {
		return err
	}
	if err := c.Retile(treeWidth); err != nil {
		return err
	}
	// Splitting moved focus into the new pane; the tree should have it back.
	if c.TreePane != "" {
		return c.Focus(c.TreePane)
	}
	return nil
}

func (c *Client) configureTabServer() error {
	tabs := c.tabClient()
	if !tabs.HasTarget(TabsSession) {
		// Nothing to configure until a connection creates it.
		return nil
	}
	opts := [][2]string{
		{"status-position", "top"},
		{"mouse", "on"},
		{"automatic-rename", "off"},
		{"remain-on-exit", "on"},
		{"history-limit", "50000"},
		{"status-left", ""},
		{"status-right", ""},
		{"window-status-format", " #W#{?window_activity_flag,•,} "},
		{"window-status-current-format", "#[reverse,bold] #W #[default]"},
	}
	for _, o := range opts {
		if _, err := tabs.run("set-option", "-g", o[0], o[1]); err != nil {
			return err
		}
	}
	return nil
}

// ensureContentPane splits the tree window once, leaving a pane to the right of
// the tree that shows the connections.
func (c *Client) ensureContentPane() error {
	out, err := c.run("list-panes", "-t", WorkspaceSession+":"+TreeWindow, "-F", "#{pane_id}")
	if err != nil {
		if noServer(err.Error()) {
			return nil
		}
		return err
	}
	if len(parseLines(out)) > 1 {
		return nil
	}
	_, err = c.run("split-window", "-h", "-t", WorkspaceSession+":"+TreeWindow,
		"sh", "-c", c.contentScript())
	return err
}

// contentPane is the pane beside the tree, i.e. the one that is not the tree's.
func (c *Client) contentPane() (string, error) {
	out, err := c.run("list-panes", "-t", WorkspaceSession+":"+TreeWindow, "-F", "#{pane_id}")
	if err != nil {
		return "", err
	}
	panes := parseLines(out)
	if len(panes) < 2 {
		return "", fmt.Errorf("the workspace has no content pane")
	}
	if c.TreePane == "" {
		return panes[len(panes)-1], nil
	}
	for _, id := range panes {
		if id != c.TreePane {
			return id, nil
		}
	}
	return "", fmt.Errorf("could not tell the content pane from the tree pane")
}

func (c *Client) focusContent() error {
	id, err := c.contentPane()
	if err != nil {
		return err
	}
	return c.Focus(id)
}

// showSidebar selects the connection's tab in the second server, creating it if
// needed, and puts the cursor in the content pane.
func (c *Client) showSidebar(slug string, argv, env []string) (string, error) {
	tabs := c.tabClient()

	panes, err := tabs.listPanes(TabsSession)
	if err != nil {
		return "", err
	}
	for _, p := range panes {
		if p.Slug != slug {
			continue
		}
		if p.Dead {
			// A dead pane cannot be revived; drop its window and rebuild.
			if _, err := tabs.run("kill-window", "-t", p.ID); err != nil {
				return "", err
			}
			break
		}
		if _, err := tabs.run("select-window", "-t", TabsSession+":"+p.Window); err != nil {
			return "", err
		}
		return p.ID, c.focusContent()
	}

	id, err := c.createTabWindow(slug, argv, env)
	if err != nil {
		return "", err
	}
	if _, err := tabs.run("select-window", "-t", TabsSession+":"+slug); err != nil {
		return "", err
	}
	return id, c.focusContent()
}

// createTabWindow opens the connection as a window in the tab server, creating
// that server's session if this is the first connection.
func (c *Client) createTabWindow(slug string, argv, env []string) (string, error) {
	tabs := c.tabClient()

	args := []string{"new-window", "-d", "-P", "-F", "#{pane_id}", "-t", TabsSession, "-n", slug}
	if !tabs.HasTarget(TabsSession) {
		args = []string{"new-session", "-d", "-P", "-F", "#{pane_id}", "-s", TabsSession, "-n", slug}
	}
	for _, e := range env {
		args = append(args, "-e", e)
	}
	args = append(args, argv...)

	out, err := tabs.run(args...)
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(out)
	if id == "" {
		return "", fmt.Errorf("tmux did not report a pane id for %s", slug)
	}
	if _, err := tabs.run("set-option", "-p", "-t", id, slugOption, slug); err != nil {
		return "", err
	}
	if _, err := tabs.run("select-pane", "-t", id, "-T", slug); err != nil {
		return "", err
	}
	// Options live on the server, so a freshly created one needs them again.
	if err := c.configureTabServer(); err != nil {
		return "", err
	}
	return id, nil
}

func parseLines(out string) []string {
	var lines []string
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	return lines
}
