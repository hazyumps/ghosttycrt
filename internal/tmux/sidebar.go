package tmux

import (
	"fmt"
	"strconv"
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

// idleArt is the wordmark shown when no connection is open. Kept short enough
// to fit the content pane at a normal tree width.
var idleArt = []string{
	" ██████╗  ██████╗ ██████╗ ████████╗",
	"██╔════╝ ██╔════╝ ██╔══██╗╚══██╔══╝",
	"██║  ███╗██║      ██████╔╝   ██║",
	"██║   ██║██║      ██╔══██╗   ██║",
	"╚██████╔╝╚██████╗ ██║  ██║   ██║",
	" ╚═════╝  ╚═════╝ ╚═╝  ╚═╝   ╚═╝",
}

// idleBlock is the empty state, centred within its own width so the shell only
// has to work out one offset.
func idleBlock() ([]string, int) {
	lines := append([]string{}, idleArt...)
	lines = append(lines,
		"",
		"no connections open yet",
		"",
		"choose one in the tree, then press enter",
	)

	width := 0
	for _, l := range lines {
		if n := len([]rune(l)); n > width {
			width = n
		}
	}
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		out = append(out, strings.Repeat(" ", (width-len([]rune(l)))/2)+l)
	}
	return out, width
}

// contentTemplate is the content pane's program. @WORD@ placeholders stand in
// for values, rather than Printf, so the shell's own % signs need no escaping.
//
// The idle screen redraws on a timer rather than once. It is drawn into a pane
// that is still settling: the split happens before the layout pins the tree to
// its width, so a single draw can land at the wrong size and stay wrong. Since
// the draw clears first, repeating it costs nothing and cannot pile up.
const contentTemplate = `draw() {
  sz=$(stty size 2>/dev/null | tr -d '\r')
  [ -n "$sz" ] || sz="24 80"
  rows=${sz%% *}
  cols=${sz##* }
  printf '\033[2J\033[H'
  if [ "$cols" -ge @W@ ]; then
    left=$(( (cols - @W@) / 2 ))
    [ "$left" -lt 0 ] && left=0
    top=$(( (rows - @N@) / 2 ))
    [ "$top" -lt 0 ] && top=0
    i=0
    while [ "$i" -lt "$top" ]; do printf '\n'; i=$((i+1)); done
    while IFS= read -r line; do
      printf '%*s%s\n' "$left" '' "$line"
    done <<'GCRT_ART'
@BLOCK@
GCRT_ART
  else
    printf '\n  no connections open\n\n  pick one in the tree\n'
  fi
}

while :; do
  if @BIN@ -L @SOCK@ has-session -t @SESS@ 2>/dev/null; then
    @BIN@ -L @SOCK@ attach -t @SESS@
    sleep 0.3
  else
    draw
    sleep 0.5
  fi
done
`

// contentScript is what the content pane runs: an empty state until a
// connection opens, then attach to the tab server's session. It retries
// because the tab server only exists while something is open.
func (c *Client) contentScript() string {
	block, width := idleBlock()
	return strings.NewReplacer(
		"@BIN@", c.Bin,
		"@SOCK@", c.tabSocket(),
		"@SESS@", TabsSession,
		"@BLOCK@", strings.Join(block, "\n"),
		"@W@", strconv.Itoa(width),
		"@N@", strconv.Itoa(len(block)),
	).Replace(contentTemplate)
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
		{"remain-on-exit", "failed"},
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

// bootstrapWindow is a placeholder that exists only long enough for the tab
// server's global options to be in force.
const bootstrapWindow = "gcrt-startup"

// createTabWindow opens the connection as a window in the tab server, creating
// that server's session if this is the first connection.
//
// The first connection is special. Options are per-server, so remain-on-exit
// has to be in force before a process starts — a command that exits at once (a
// mistyped host, say) would otherwise die before the option was set and take
// its window with it. So the session is brought up with a placeholder window,
// the options are applied, and that window is *respawned* into the connection.
// Respawning rather than killing-and-creating keeps the window count at one and
// sidesteps tmux refusing to kill a session's last window.
func (c *Client) createTabWindow(slug string, argv, env []string) (string, error) {
	tabs := c.tabClient()

	if !tabs.HasTarget(TabsSession) {
		if _, err := tabs.run("new-session", "-d", "-s", TabsSession,
			"-n", bootstrapWindow, "sleep", "86400"); err != nil {
			return "", err
		}
		if err := c.configureTabServer(); err != nil {
			return "", err
		}

		args := []string{"respawn-window", "-k", "-t", TabsSession + ":" + bootstrapWindow}
		for _, e := range env {
			args = append(args, "-e", e)
		}
		args = append(args, argv...)
		if _, err := tabs.run(args...); err != nil {
			return "", err
		}
		if _, err := tabs.run("rename-window", "-t", TabsSession+":"+bootstrapWindow, slug); err != nil {
			return "", err
		}
		out, err := tabs.run("list-panes", "-t", TabsSession+":"+slug, "-F", "#{pane_id}")
		if err != nil {
			return "", err
		}
		panes := parseLines(out)
		if len(panes) == 0 {
			return "", fmt.Errorf("tmux did not report a pane id for %s", slug)
		}
		return panes[0], c.tagPane(tabs, panes[0], slug)
	}

	if err := c.configureTabServer(); err != nil {
		return "", err
	}

	args := []string{"new-window", "-d", "-P", "-F", "#{pane_id}", "-t", TabsSession, "-n", slug}
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
	return id, c.tagPane(tabs, id, slug)
}

// tagPane records which session a pane belongs to, and labels it.
func (c *Client) tagPane(tabs *Client, paneID, slug string) error {
	if _, err := tabs.run("set-option", "-p", "-t", paneID, slugOption, slug); err != nil {
		return err
	}
	_, err := tabs.run("select-pane", "-t", paneID, "-T", slug)
	return err
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
