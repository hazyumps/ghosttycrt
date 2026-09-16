package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/hazyumps/ghosttycrt/internal/session"
	"github.com/hazyumps/ghosttycrt/internal/transport"
)

var (
	colAccent = lipgloss.AdaptiveColor{Light: "#663399", Dark: "#c9a0ff"}
	colDim    = lipgloss.AdaptiveColor{Light: "#8a8a8a", Dark: "#6c6c6c"}
	colText   = lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#e4e4e4"}
	colOK     = lipgloss.AdaptiveColor{Light: "#2f7d32", Dark: "#7bd88f"}
	colWarn   = lipgloss.AdaptiveColor{Light: "#a06000", Dark: "#ffb454"}
	colErr    = lipgloss.AdaptiveColor{Light: "#a02020", Dark: "#ff6b6b"}

	styleTitle  = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	styleDim    = lipgloss.NewStyle().Foreground(colDim)
	styleText   = lipgloss.NewStyle().Foreground(colText)
	styleGroup  = lipgloss.NewStyle().Foreground(colAccent)
	styleOK     = lipgloss.NewStyle().Foreground(colOK)
	styleWarn   = lipgloss.NewStyle().Foreground(colWarn)
	styleErr    = lipgloss.NewStyle().Foreground(colErr)
	styleCursor = lipgloss.NewStyle().Foreground(colText).Background(lipgloss.AdaptiveColor{Light: "#e0d6f5", Dark: "#3a2f52"})
	styleLabel  = lipgloss.NewStyle().Foreground(colDim).Width(13)
	styleBox    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2)
)

func (m *Model) View() string {
	if m.width == 0 {
		return "loading…"
	}
	switch {
	case m.confirm != nil:
		return m.viewConfirm()
	case m.menu != nil:
		return m.viewMenu()
	case m.help:
		return m.viewHelp()
	}
	if len(m.file.Session) == 0 {
		return m.viewEmpty()
	}
	return lipgloss.JoinVertical(lipgloss.Left, m.viewHeader(), m.viewBody(), m.viewFooter())
}

func (m *Model) viewHeader() string {
	left := styleTitle.Render(" ghosttycrt ")
	if m.filtering {
		left += styleDim.Render("filter: ") + styleText.Render("/"+m.filter+"█")
	} else if m.filter != "" {
		left += styleDim.Render("filter: ") + styleText.Render("/"+m.filter)
	}
	right := styleDim.Render(joinCaps(m.Capabilities()) + " ")
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m *Model) viewBody() string {
	height := m.height - 2
	if height < 3 {
		height = 3
	}
	if m.width < 70 {
		return pad(m.viewTree(m.width, height), m.width, height)
	}
	treeW := m.width * 2 / 5
	if treeW < 24 {
		treeW = 24
	}
	detailW := m.width - treeW - 1
	return lipgloss.JoinHorizontal(lipgloss.Top, m.viewTree(treeW, height), " ", m.viewDetails(detailW, height))
}

func (m *Model) viewTree(width, height int) string {
	lines := make([]string, 0, len(m.rows))
	for i, row := range m.rows {
		line := m.renderRow(row)
		if i == m.cursor {
			line = styleCursor.Width(width).Render(line)
		}
		lines = append(lines, clip(line, width))
	}
	return pad(strings.Join(lines, "\n"), width, height)
}

func (m *Model) renderRow(row session.Row) string {
	node := row.Node
	if node.Kind == session.KindGroup {
		glyph := "▶"
		if row.Expanded {
			glyph = "▼"
		}
		return row.Prefix + styleGroup.Render(glyph+" "+node.Label)
	}

	pin := " "
	if node.Session.Pinned {
		pin = "★"
	}
	return row.Prefix + m.statusGlyph(node.Session) + " " + styleDim.Render(pin) + " " + styleText.Render(node.Label)
}

// statusGlyph mirrors tmux state: attached, running, not running, or a session
// whose transport cannot work at all.
func (m *Model) statusGlyph(s *session.Session) string {
	if st, ok := m.live[s.Slug]; ok {
		if st.Attached > 0 {
			return styleOK.Render("●")
		}
		return styleGroup.Render("○")
	}
	if _, err := transport.For(s, m.cfg); err != nil {
		return styleWarn.Render("✗")
	}
	return styleDim.Render("·")
}

func (m *Model) statusText(s *session.Session) string {
	st, ok := m.live[s.Slug]
	if !ok {
		if _, err := transport.For(s, m.cfg); err != nil {
			return "unavailable"
		}
		return "not running"
	}
	if st.Attached > 0 {
		return "attached"
	}
	return "running, detached"
}

func (m *Model) viewDetails(width, height int) string {
	s := m.selectedSession()
	if s == nil {
		return pad(styleDim.Render("select a session"), width, height)
	}

	lines := []string{
		styleTitle.Render(s.Name),
		styleDim.Render(strings.Repeat("─", max(1, width-1))),
		kv("transport", string(s.Transport)),
	}
	if ep := s.Endpoint(); ep != "" {
		lines = append(lines, kv("endpoint", ep))
	}
	lines = append(lines,
		kv("group", orDash(s.Group)),
		kv("credential", s.CredentialRef()),
		kv("logging", loggingSummary(s, m.cfg.Logging.EnabledByDefault)),
		kv("state", m.statusText(s)),
	)
	if len(s.Tags) > 0 {
		lines = append(lines, kv("tags", strings.Join(s.Tags, ", ")))
	}
	if s.Description != "" {
		lines = append(lines, kv("description", s.Description))
	}
	if _, err := transport.For(s, m.cfg); err != nil {
		lines = append(lines, "", styleWarn.Render("! "+err.Error()))
	}

	if len(m.problems.Errors) > 0 || len(m.problems.Warnings) > 0 {
		lines = append(lines, "")
		for _, e := range m.problems.Errors {
			lines = append(lines, styleErr.Render("✗ "+e.Error()))
		}
		for _, w := range m.problems.Warnings {
			lines = append(lines, styleWarn.Render("! "+w.Error()))
		}
	}

	body := make([]string, 0, height)
	for _, l := range lines {
		body = append(body, clip(l, width))
	}
	return pad(strings.Join(body, "\n"), width, height)
}

func (m *Model) viewFooter() string {
	left := styleDim.Render(" " + "enter:connect  d:detach/kill  n:new  e:edit  l:log  /:filter  r:refresh  ?:help")
	switch {
	case m.errMsg != "":
		left = styleErr.Render(" ✗ " + m.errMsg)
	case m.status != "":
		left = styleText.Render(" " + m.status)
	}
	right := styleDim.Render("Ctrl-b d returns to the tree ")
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	line := left + strings.Repeat(" ", gap) + right

	if m.filtering {
		return styleDim.Render(" type to filter  enter:keep  esc:clear")
	}
	return line
}

func (m *Model) viewEmpty() string {
	lines := []string{
		"",
		styleTitle.Render("No sessions yet."),
		"",
		styleText.Render("  n   add one"),
		styleText.Render("  i   import from ~/.ssh/config"),
		"",
		styleDim.Render("  config  " + m.paths.SessionsFile()),
		"",
		styleDim.Render("  q   quit"),
	}
	if len(m.problems.Errors) > 0 {
		lines = append(lines, "")
		for _, e := range m.problems.Errors {
			lines = append(lines, styleErr.Render("  ✗ "+e.Error()))
		}
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (m *Model) viewMenu() string {
	lines := []string{
		styleTitle.Render(m.menu.title),
		styleDim.Render("choose an action"),
		"",
	}
	for i, item := range m.menu.items {
		cursor := "  "
		if i == m.menu.index {
			cursor = styleTitle.Render("▸ ")
		}
		label := item.label
		if item.destructive {
			label = styleWarn.Render(label)
		} else {
			label = styleText.Render(label)
		}
		lines = append(lines, cursor+label+"  "+styleDim.Render(item.hint))
	}
	lines = append(lines, "", styleDim.Render("j/k move   enter select   esc cancel"))
	return m.center(lines)
}

func (m *Model) viewConfirm() string {
	lines := []string{
		styleWarn.Render("Confirm"),
		"",
		styleText.Render(m.confirm.prompt),
		"",
		styleDim.Render("y: yes      N: no (default)"),
	}
	return m.center(lines)
}

func (m *Model) viewHelp() string {
	binds := [][2]string{
		{"↑ ↓ / j k", "move"},
		{"← →", "collapse / expand group"},
		{"enter", "connect (attach-or-create)"},
		{"d", "detach or kill the selected session"},
		{"/", "filter over name, host, group, tags"},
		{"esc", "clear filter"},
		{"r", "refresh tmux state"},
		{"?", "toggle this help"},
		{"q", "quit"},
	}
	lines := []string{styleTitle.Render("ghosttycrt — help"), ""}
	for _, b := range binds {
		lines = append(lines, "  "+styleText.Width(14).Render(b[0])+styleDim.Render(b[1]))
	}
	lines = append(lines, "", styleDim.Render("  "+m.paths.SessionsFile()))
	return m.center(lines)
}

func (m *Model) center(lines []string) string {
	box := styleBox.Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func kv(k, v string) string {
	return styleLabel.Render(k) + styleText.Render(v)
}

func loggingSummary(s *session.Session, def bool) string {
	if s.LoggingEnabled(def) {
		return "on"
	}
	return "off"
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func pad(s string, width, height int) string {
	lines := strings.Split(s, "\n")
	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	for i := range lines {
		lines[i] = clip(lines[i], width)
	}
	return strings.Join(lines, "\n")
}

func clip(s string, width int) string {
	if lipgloss.Width(s) <= width {
		return s
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(s)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
