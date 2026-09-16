package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hazyumps/ghosttycrt/internal/config"
	"github.com/hazyumps/ghosttycrt/internal/session"
	"github.com/hazyumps/ghosttycrt/internal/tmux"
)

type Model struct {
	cfg      *config.Config
	file     *session.File
	paths    config.Paths
	problems session.Problems

	root      *session.Node
	rows      []session.Row
	cursor    int
	collapsed map[string]bool

	filter    string
	filtering bool
	showHelp  bool

	live   map[string]tmux.SessionState
	tmuxOK bool

	width  int
	height int
	status string
	quit   bool
}

func New(cfg *config.Config, file *session.File, paths config.Paths, problems session.Problems) Model {
	m := Model{
		cfg:       cfg,
		file:      file,
		paths:     paths,
		problems:  problems,
		collapsed: map[string]bool{},
		live:      map[string]tmux.SessionState{},
		tmuxOK:    tmux.Available(),
	}
	m.rebuild()
	m.cursorToFirstSession()
	return m
}

// cursorToFirstSession puts the highlight on a connectable row rather than a
// group header, which is where a user expects to land on startup.
func (m *Model) cursorToFirstSession() {
	for i, r := range m.rows {
		if r.Node.Kind == session.KindSession {
			m.cursor = i
			return
		}
	}
}

func (m Model) Init() tea.Cmd { return nil }

func (m *Model) rebuild() {
	m.root = session.BuildTree(m.file.Session)
	m.rows = m.root.Flatten(m.collapsed, m.filter)
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m Model) selected() *session.Row {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return nil
	}
	return &m.rows[m.cursor]
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if m.width == 0 {
			m.width = 80
		}
		if m.height == 0 {
			m.height = 24
		}
		return m, nil

	case tea.KeyMsg:
		if m.showHelp {
			switch msg.String() {
			case "?", "esc", "q", "enter":
				m.showHelp = false
			}
			return m, nil
		}
		if m.filtering {
			return m.updateFilter(msg)
		}
		return m.updateNormal(msg)
	}
	return m, nil
}

func (m Model) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quit = true
		return m, tea.Quit
	case "esc":
		m.filtering = false
		m.filter = ""
		m.rebuild()
		m.cursorToFirstSession()
	case "enter":
		m.filtering = false
	case "backspace":
		if r := []rune(m.filter); len(r) > 0 {
			m.filter = string(r[:len(r)-1])
			m.rebuild()
			m.cursorToFirstSession()
		}
	default:
		if s := msg.String(); len(s) == 1 {
			m.filter += s
			m.rebuild()
			m.cursorToFirstSession()
		}
	}
	return m, nil
}

func (m Model) updateNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		m.quit = true
		return m, tea.Quit
	case "?":
		m.showHelp = true
	case "/":
		m.filtering = true
	case "esc":
		if m.filter != "" {
			m.filter = ""
			m.rebuild()
			m.cursorToFirstSession()
		}
	case "j", "down":
		if m.cursor < len(m.rows)-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "g":
		m.cursor = 0
	case "G":
		if len(m.rows) > 0 {
			m.cursor = len(m.rows) - 1
		}
	case "ctrl+r", "r":
		m.refresh()
	case "right", "l":
		m.expand()
	case "left", "h":
		m.collapse()
	}
	return m, nil
}

func (m *Model) expand() {
	row := m.selected()
	if row == nil || row.Node.Kind != session.KindGroup {
		return
	}
	delete(m.collapsed, row.Node.Path)
	m.rebuild()
}

func (m *Model) collapse() {
	row := m.selected()
	if row == nil {
		return
	}
	if row.Node.Kind == session.KindGroup && !m.collapsed[row.Node.Path] {
		m.collapsed[row.Node.Path] = true
		m.rebuild()
		return
	}
	for i := m.cursor - 1; i >= 0; i-- {
		if m.rows[i].Node.Kind == session.KindGroup && m.rows[i].Depth < row.Depth {
			m.cursor = i
			return
		}
	}
}

func (m *Model) refresh() {
	m.tmuxOK = tmux.Available()
	m.live = map[string]tmux.SessionState{}
	if m.tmuxOK {
		if bySlug, err := tmux.BySlug(tmux.Socket); err == nil {
			m.live = bySlug
		}
	}
	m.rebuild()
}

func (m Model) Filter() string { return m.filter }

func (m Model) Capabilities() []string {
	caps := []string{}
	if m.tmuxOK {
		caps = append(caps, "tmux ✓")
	} else {
		caps = append(caps, "tmux ✗")
	}
	caps = append(caps, "vault "+m.cfg.Credentials.DefaultProvider)
	caps = append(caps, "picocom "+present("picocom"))
	caps = append(caps, "ssh "+present("ssh"))
	return caps
}

func (m Model) SessionCount() int {
	return len(m.file.Session)
}

func present(bin string) string {
	if _, err := lookPath(bin); err != nil {
		return "✗"
	}
	return "✓"
}

func joinCaps(caps []string) string { return strings.Join(caps, "  ") }
