package tui

import (
	"os/exec"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/hazyumps/ghosttycrt/internal/session"
)

func lookPath(bin string) (string, error) { return exec.LookPath(bin) }

var (
	colAccent = lipgloss.AdaptiveColor{Light: "#663399", Dark: "#c9a0ff"}
	colDim    = lipgloss.AdaptiveColor{Light: "#8a8a8a", Dark: "#6c6c6c"}
	colText   = lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#e4e4e4"}
	colWarn   = lipgloss.AdaptiveColor{Light: "#a06000", Dark: "#ffb454"}
	colErr    = lipgloss.AdaptiveColor{Light: "#a02020", Dark: "#ff6b6b"}

	styleTitle  = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	styleDim    = lipgloss.NewStyle().Foreground(colDim)
	styleText   = lipgloss.NewStyle().Foreground(colText)
	styleGroup  = lipgloss.NewStyle().Foreground(colAccent)
	styleCursor = lipgloss.NewStyle().Foreground(colText).Background(lipgloss.AdaptiveColor{Light: "#e0d6f5", Dark: "#3a2f52"})
	styleWarn   = lipgloss.NewStyle().Foreground(colWarn)
	styleErr    = lipgloss.NewStyle().Foreground(colErr)
	styleLabel  = lipgloss.NewStyle().Foreground(colDim).Width(13)
)

func (m Model) View() string {
	if m.width == 0 {
		return "loading…"
	}
	if m.showHelp {
		return m.viewHelp()
	}
	if len(m.file.Session) == 0 {
		return m.viewEmpty()
	}

	header := m.viewHeader()
	body := m.viewBody()
	footer := m.viewFooter()

	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

func (m Model) viewHeader() string {
	label := "filter: "
	prompt := "/"
	if m.filtering {
		prompt = "/" + m.filter + "█"
	} else if m.filter != "" {
		prompt = "/" + m.filter
	} else {
		prompt = ""
	}
	left := styleTitle.Render(" ghosttycrt ") + styleDim.Render(label) + styleText.Render(prompt)
	right := styleDim.Render(joinCaps(m.Capabilities()) + " ")
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m Model) viewBody() string {
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
	left := m.viewTree(treeW, height)
	right := m.viewDetails(detailW, height)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)
}

func (m Model) viewTree(width, height int) string {
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

func (m Model) renderRow(row session.Row) string {
	node := row.Node
	var glyph, label string
	switch node.Kind {
	case session.KindGroup:
		if row.Expanded {
			glyph = "▼"
		} else {
			glyph = "▶"
		}
		label = node.Label
		return row.Prefix + styleGroup.Render(glyph+" "+label)
	default:
		glyph = "·"
		if node.Session.Pinned {
			glyph = "★"
		}
		label = node.Label
		return row.Prefix + styleDim.Render(glyph+" ") + styleText.Render(label)
	}
}

func (m Model) viewDetails(width, height int) string {
	row := m.selected()
	if row == nil || row.Node.Kind != session.KindSession {
		return pad(styleDim.Render("select a session"), width, height)
	}
	s := row.Node.Session

	lines := []string{
		styleTitle.Render(s.Name),
		styleDim.Render(strings.Repeat("─", max(1, width-1))),
	}
	lines = append(lines, kv("transport", string(s.Transport)))
	if ep := s.Endpoint(); ep != "" {
		lines = append(lines, kv("endpoint", ep))
	}
	lines = append(lines, kv("group", orDash(s.Group)))
	lines = append(lines, kv("credential", s.CredentialRef()))
	lines = append(lines, kv("logging", loggingSummary(s, m.cfg.Logging.EnabledByDefault)))
	if len(s.Tags) > 0 {
		lines = append(lines, kv("tags", strings.Join(s.Tags, ", ")))
	}
	if s.Description != "" {
		lines = append(lines, kv("description", s.Description))
	}
	if st, ok := m.live[s.Slug]; ok {
		state := "running"
		if st.Attached > 0 {
			state = "attached"
		}
		lines = append(lines, kv("tmux", state))
	} else if m.tmuxOK {
		lines = append(lines, kv("tmux", "not running"))
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

func (m Model) viewFooter() string {
	keys := "enter:attach  n:new  e:edit  d:kill  l:log  L:logs  r:refresh  ?:help"
	if m.filtering {
		keys = "type to filter  enter:keep  esc:clear"
	}
	left := styleDim.Render(" " + keys)
	right := styleDim.Render("Ctrl-b d returns to the tree ")
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m Model) viewEmpty() string {
	msg := []string{
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
		msg = append(msg, "")
		for _, e := range m.problems.Errors {
			msg = append(msg, styleErr.Render("  ✗ "+e.Error()))
		}
	}
	return lipgloss.JoinVertical(lipgloss.Left, msg...)
}

func (m Model) viewHelp() string {
	binds := [][2]string{
		{"↑ ↓ / j k", "move"},
		{"← →", "collapse / expand group"},
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
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
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
