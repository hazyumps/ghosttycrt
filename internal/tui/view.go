package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
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
	styleAccent = lipgloss.NewStyle().Foreground(colAccent)
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

// barItem is one clickable item in the header menu bar. start and end are
// screen columns, filled in by renderBar so a click can be hit-tested against
// exactly what was drawn.
type barItem struct {
	label    string
	title    string
	emphasis bool
	start    int
	end      int
	run      func() tea.Cmd
}

func (m *Model) barActions() []barItem {
	items := []barItem{
		{label: "?", title: "Help", emphasis: true, run: func() tea.Cmd {
			m.help = true
			m.helpTop = 0
			return nil
		}},
		{label: "/", title: "Filter", run: func() tea.Cmd { m.filtering = true; return nil }},
		{label: "r", title: "Refresh", run: func() tea.Cmd { m.refresh(); m.status = "refreshed"; return nil }},
	}
	if m.workspace {
		items = append(items, barItem{label: "q", title: "Detach", run: func() tea.Cmd {
			return m.detachWorkspace()
		}})
	} else {
		items = append(items, barItem{label: "q", title: "Quit", run: m.requestQuit})
	}
	return items
}

// renderBar draws the menu bar and reports where each item landed. Rendering
// and hit-testing share this function so they cannot drift apart.
func (m *Model) renderBar() (string, []barItem) {
	wide := m.width >= 50
	brand := " gcrt "
	if wide {
		brand = " ghosttycrt "
	}

	var b strings.Builder
	b.WriteString(styleTitle.Render(brand))
	col := lipgloss.Width(brand)

	items := m.barActions()
	for i := range items {
		b.WriteString("  ")
		col += 2

		label := items[i].label
		if wide {
			label = items[i].title
		}
		style := styleText
		if items[i].emphasis {
			style = styleAccent
		}
		b.WriteString(style.Render(label))

		items[i].start = col
		col += lipgloss.Width(label)
		items[i].end = col
	}
	return b.String(), items
}

func (m *Model) viewHeader() string {
	bar, _ := m.renderBar()

	filter := ""
	if m.filter != "" || m.filtering {
		filter = "/" + m.filter
		if m.filtering {
			filter += "█"
		}
		filter = "  " + styleText.Render(filter)
	}

	if m.width < 50 {
		right := ""
		if len(m.rows) > m.bodyHeight() {
			right = fmt.Sprintf("%d/%d ", m.cursor+1, len(m.rows))
		}
		return clip(justify(bar+filter, styleDim.Render(right), m.width), m.width)
	}

	scroll := ""
	if len(m.rows) > m.bodyHeight() {
		end := m.offset + m.bodyHeight()
		if end > len(m.rows) {
			end = len(m.rows)
		}
		scroll = styleDim.Render(fmt.Sprintf(" %d–%d/%d ", m.offset+1, end, len(m.rows)))
	}
	caps := styleDim.Render(joinCaps(m.Capabilities()) + " ")
	return m.fitLine(bar+filter, scroll, caps)
}

func justify(left, right string, width int) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		gap = 0
	}
	return left + strings.Repeat(" ", gap) + right
}

// fitLine puts the scroll position and the capability strip at the far side
// when they fit, degrading rather than overflowing: a composed line longer than
// the pane wraps and wrecks the layout, so something has to go. The capability
// strip is dropped first — with a long tree the scroll position matters more —
// then the scroll indicator, then the left side is clipped.
func (m *Model) fitLine(left, scroll, caps string) string {
	if lipgloss.Width(left)+lipgloss.Width(scroll)+lipgloss.Width(caps) < m.width {
		return justify(left, scroll+caps, m.width)
	}
	if lipgloss.Width(left)+lipgloss.Width(scroll) < m.width {
		return justify(left, scroll, m.width)
	}
	if lipgloss.Width(left)+lipgloss.Width(caps) < m.width {
		return justify(left, caps, m.width)
	}
	if lipgloss.Width(left) < m.width {
		return justify(left, "", m.width)
	}
	return clip(left, m.width)
}

func (m *Model) viewBody() string {
	height := m.bodyHeight()
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
	lines := make([]string, 0, height)
	end := m.offset + height
	if end > len(m.rows) {
		end = len(m.rows)
	}
	for i := m.offset; i < end; i++ {
		line := m.renderRow(m.rows[i])
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

// statusGlyph mirrors real state: in workspace mode a connection is open beside
// the tree, running in the background, or gone; inline it is attached, running,
// not running, or a session whose transport cannot work at all.
func (m *Model) statusGlyph(s *session.Session) string {
	if m.workspace {
		if p, ok := m.panes[s.Slug]; ok {
			switch {
			case p.Dead:
				return styleErr.Render("✗")
			case p.Visible():
				return styleOK.Render("●")
			default:
				return styleGroup.Render("○")
			}
		}
	} else if st, ok := m.live[s.Slug]; ok {
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
	if m.workspace {
		if p, ok := m.panes[s.Slug]; ok {
			switch {
			case p.Dead:
				return fmt.Sprintf("exited (status %d)", p.Exit)
			case p.Visible():
				return "open beside the tree"
			default:
				return "running, hidden"
			}
		}
	} else if st, ok := m.live[s.Slug]; ok {
		if st.Attached > 0 {
			return "attached"
		}
		return "running, detached"
	}
	if _, err := transport.For(s, m.cfg); err != nil {
		return "unavailable"
	}
	return "not running"
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
	if m.width < 50 {
		switch {
		case m.errMsg != "":
			return clip(styleErr.Render(" ✗ "+m.errMsg), m.width)
		case m.status != "":
			return clip(styleText.Render(" "+m.status), m.width)
		case m.filtering:
			return clip(styleDim.Render(" type to filter  enter keep  esc clear"), m.width)
		case m.workspace:
			return clip(styleDim.Render(" enter open  d kill  / filter  q detach"), m.width)
		default:
			return clip(styleDim.Render(" enter connect  d kill  / filter  q quit"), m.width)
		}
	}

	keys := "enter:connect  d:detach/kill  n:new  e:edit  l:log  /:filter  r:refresh  ?:help"
	back := "Ctrl-b d returns to the tree "
	if m.workspace {
		keys = "enter:open  d:show/hide/kill  n:new  e:edit  l:log  /:filter  r:refresh  ?:help"
		back = "Ctrl-b t returns to the tree "
	}
	left := styleDim.Render(" " + keys)
	switch {
	case m.errMsg != "":
		left = styleErr.Render(" ✗ " + m.errMsg)
	case m.status != "":
		left = styleText.Render(" " + m.status)
	}
	if m.filtering {
		return clip(styleDim.Render(" type to filter  enter:keep  esc:clear"), m.width)
	}
	return m.fitLine(left, "", styleDim.Render(back))
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
	inner := m.innerWidth()
	lines := []string{styleDim.Render(clip("choose an action", inner))}
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
		line := cursor + label
		// Hints only when there is room for them.
		if inner >= 34 && item.hint != "" {
			line += "  " + styleDim.Render(item.hint)
		}
		lines = append(lines, clip(line, inner))
	}
	lines = append(lines, "", styleDim.Render(clip("j/k move   enter select   esc cancel", inner)))
	return m.modal(m.menu.title, lines)
}

func (m *Model) viewConfirm() string {
	inner := m.innerWidth()
	lines := []string{styleWarn.Render("Confirm"), ""}
	for _, l := range wrap(m.confirm.prompt, inner) {
		lines = append(lines, styleText.Render(clip(l, inner)))
	}
	lines = append(lines, "", styleDim.Render(clip("y: yes      N: no (default)", inner)))
	return m.modal("", lines)
}

type helpSection struct {
	title string
	rows  [][2]string
}

func (m *Model) helpSections() []helpSection {
	sections := []helpSection{
		{title: "Moving", rows: [][2]string{
			{"↑ ↓ / j k", "move"},
			{"← →", "collapse / expand a group"},
			{"pgup / pgdn", "page through a long tree"},
			{"g / G", "jump to the top / bottom"},
			{"/", "filter by name, host, group or tag"},
			{"esc", "clear the filter"},
		}},
		{title: "Sessions", rows: [][2]string{
			{"enter", "connect (attach-or-create)"},
			{"d", "detach, hide or kill — a menu"},
			{"r", "refresh tmux state"},
			{"n / e / c", "new / edit / duplicate — M2, not yet"},
			{"space", "pin or unpin — M2, not yet"},
			{"l / L", "today's log / log browser — M3, not yet"},
		}},
		{title: "Mouse", rows: [][2]string{
			{"click a host", "open it"},
			{"click a group", "fold or unfold it"},
			{"click the bar", "the menu items along the top"},
			{"wheel", "scroll the tree"},
		}},
	}

	if m.workspace {
		sections = append([]helpSection{{title: "Workspace", rows: [][2]string{
			{"Ctrl-b t", "back to the tree from a session"},
			{"Ctrl-b d", "detach the whole workspace"},
			{"Ctrl-b z", "zoom a pane to fill the window"},
		}}}, sections...)
	}

	sections = append(sections, helpSection{title: "Commands", rows: [][2]string{
		{"gcrt", "start the tree"},
		{"gcrt check", "validate config and sessions"},
		{"gcrt import", "import SecureCRT sessions"},
	}})

	return sections
}

// helpContent is the whole help screen as styled lines, before scrolling.
func (m *Model) helpContent() []string {
	inner := m.innerWidth()
	wide := inner >= 30

	var lines []string
	for i, sec := range m.helpSections() {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, styleAccent.Render(clip(sec.title, inner)))
		for _, row := range sec.rows {
			if wide {
				// clip the key first: Width is a minimum, not a maximum, so a
				// long key would push the description past the box edge.
				lines = append(lines, "  "+styleText.Width(20).Render(clip(row[0], 18))+
					styleDim.Render(clip(row[1], inner-22)))
				continue
			}
			// Narrow: the key on its own line, the description wrapped beneath.
			lines = append(lines, "  "+styleText.Render(clip(row[0], inner)))
			for _, l := range wrap(row[1], inner-4) {
				lines = append(lines, "    "+styleDim.Render(clip(l, inner-4)))
			}
		}
	}
	return lines
}

// helpWindow is how many content lines fit in the box.
func (m *Model) helpWindow() int {
	n := m.innerHeight() - 3 // title, blank, hint
	if n < 1 {
		n = 1
	}
	return n
}

func (m *Model) scrollHelp(delta int) {
	total := len(m.helpContent())
	max := total - m.helpWindow()
	if max < 0 {
		max = 0
	}
	m.helpTop += delta
	if m.helpTop > max {
		m.helpTop = max
	}
	if m.helpTop < 0 {
		m.helpTop = 0
	}
}

func (m *Model) viewHelp() string {
	inner := m.innerWidth()
	content := m.helpContent()
	window := m.helpWindow()

	m.scrollHelp(0) // clamp after a resize
	end := m.helpTop + window
	if end > len(content) {
		end = len(content)
	}
	visible := content[m.helpTop:end]

	hint := "j/k or wheel: scroll   esc: close"
	if m.helpTop > 0 && end < len(content) {
		hint = "▲▼  " + hint
	} else if end < len(content) {
		hint = "▼ more   " + hint
	} else if m.helpTop > 0 {
		hint = "▲ more   " + hint
	}
	visible = append(visible, "", styleDim.Render(clip(hint, inner)))

	return m.modal("ghosttycrt — help", visible)
}

// innerWidth is how wide modal content may be: the pane minus the box border
// (2) and its horizontal padding (4).
func (m *Model) innerWidth() int {
	w := m.width - 6
	if w < 8 {
		w = 8
	}
	return w
}

// innerHeight is the same for rows: a box taller than the pane overflows and
// blanks whatever is behind it, exactly as an over-wide one does.
func (m *Model) innerHeight() int {
	h := m.height - 4
	if h < 1 {
		h = 1
	}
	return h
}

// modal draws a bordered box that is guaranteed to fit the pane. Content is
// clipped to innerWidth and truncated to innerHeight first, so the box can
// never be larger than the terminal — an oversized box spills into the
// neighbouring pane, because Place cannot shrink what it is handed.
func (m *Model) modal(title string, lines []string) string {
	inner := m.innerWidth()
	height := m.innerHeight()

	content := make([]string, 0, len(lines)+1)
	if title != "" {
		content = append(content, styleTitle.Render(clip(title, inner)))
	}
	for _, l := range lines {
		content = append(content, clip(l, inner))
	}
	if len(content) > height {
		content = content[:height]
	}

	box := styleBox.Render(lipgloss.JoinVertical(lipgloss.Left, content...))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

// wrap breaks plain text on spaces to fit width.
func wrap(s string, width int) []string {
	if width < 1 {
		return []string{s}
	}
	var out []string
	line := ""
	for _, word := range strings.Fields(s) {
		switch {
		case line == "":
			line = word
		case len(line)+1+len(word) <= width:
			line += " " + word
		default:
			out = append(out, line)
			line = word
		}
	}
	if line != "" {
		out = append(out, line)
	}
	if len(out) == 0 {
		out = []string{""}
	}
	return out
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
