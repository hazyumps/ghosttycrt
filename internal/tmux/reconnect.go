package tmux

func (c *Client) configureConnectionMenu() error {
	// -O makes this a click-to-choose menu instead of a press-and-drag gesture:
	// releasing the opening click or moving outside must not dismiss it. -M
	// also keeps confirmations mouse-active when reached by a keyboard shortcut.
	return c.runQuiet("bind-key", "-T", "root", "MouseDown3Status",
		"display-menu", "-O", "-M", "-T", "#[align=centre]#{window_name}", "-t", "=", "-x", "W", "-y", "W",
		"Switch to it", "s", "select-window",
		"#{?@gcrt_slug,Reconnect,-Reconnect}", "R", `if-shell -F '#{pane_dead}' 'respawn-pane' 'display-menu -O -M -T "Reconnect #W? Stops current work" -x C -y C -C 0 "No (cancel)" n "" "Yes, reconnect" y "respawn-pane -k"'`,
		"#{?@gcrt_slug,Close,-Close}", "X", `display-menu -O -M -T "Close #W and stop its process?" -x C -y C -C 0 "No (cancel)" n "" "Yes, close" y "kill-window"`)
}

// Reconnect restarts only this connection, keeping its pane and the others.
// force may be true only after confirmation; without it tmux refuses to stop
// a process that became live between the status check and this command.
func (c *Client) Reconnect(slug string, argv, env []string, width int, force bool) (string, error) {
	panes, err := c.Panes()
	if err != nil {
		return "", err
	}
	for _, p := range panes {
		if p.Slug != slug {
			continue
		}
		host := c
		if c.layout() == LayoutSidebar {
			host = c.tabClient()
		}
		if err := host.respawnConnection(p.ID, argv, env, force); err != nil {
			return "", err
		}
		switch c.layout() {
		case LayoutSidebar:
			if err := host.runQuiet("select-window", "-t", p.ID); err != nil {
				return "", err
			}
			return p.ID, c.focusContent()
		case LayoutTabs:
			return p.ID, c.runQuiet("select-window", "-t", p.ID)
		default:
			if !p.Visible() {
				if err := c.runQuiet("join-pane", "-d", "-s", p.ID, "-t", WorkspaceSession+":"+TreeWindow); err != nil {
					return "", err
				}
				if err := c.Retile(width); err != nil {
					return "", err
				}
			}
			return p.ID, c.Focus(p.ID)
		}
	}
	return c.Show(slug, argv, env, width)
}

func (c *Client) ReconnectSession(slug string, argv, env []string, opts map[string]string, force bool) error {
	if !c.Has(slug) {
		return c.Ensure(slug, argv, env, opts)
	}
	panes, err := c.listPanes(SessionName(slug))
	if err != nil {
		return err
	}
	if len(panes) == 0 {
		return c.Ensure(slug, argv, env, opts)
	}
	return c.respawnConnection(panes[0].ID, argv, env, force)
}

func (c *Client) respawnConnection(id string, argv, env []string, force bool) error {
	args := []string{"respawn-pane", "-t", id}
	if force {
		args = append(args, "-k")
	}
	for _, value := range env {
		args = append(args, "-e", value)
	}
	return c.runQuiet(append(args, argv...)...)
}
