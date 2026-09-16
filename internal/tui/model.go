package tui

import (
	"fmt"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hazyumps/ghosttycrt/internal/config"
	"github.com/hazyumps/ghosttycrt/internal/session"
	"github.com/hazyumps/ghosttycrt/internal/tmux"
	"github.com/hazyumps/ghosttycrt/internal/transport"
)

type Model struct {
	cfg      *config.Config
	file     *session.File
	paths    config.Paths
	problems session.Problems
	client   *tmux.Client

	root      *session.Node
	rows      []session.Row
	cursor    int
	collapsed map[string]bool

	filter    string
	filtering bool

	live map[string]tmux.SessionState

	help    bool
	menu    *menuState
	confirm *confirmState

	status string
	errMsg string

	width  int
	height int
}

type menuItem struct {
	label       string
	hint        string
	destructive bool
	run         func() tea.Cmd
}

type menuState struct {
	title string
	items []menuItem
	index int
}

type confirmState struct {
	prompt string
	yes    func() tea.Cmd
}

type attachReadyMsg struct{ cmd *exec.Cmd }
type attachFailedMsg struct{ err error }
type attachDoneMsg struct{ err error }

func New(cfg *config.Config, file *session.File, paths config.Paths, problems session.Problems, client *tmux.Client) *Model {
	m := &Model{
		cfg:       cfg,
		file:      file,
		paths:     paths,
		problems:  problems,
		client:    client,
		collapsed: map[string]bool{},
		live:      map[string]tmux.SessionState{},
	}
	m.rebuild()
	m.cursorToFirstSession()
	m.refresh()
	return m
}

func (m *Model) Init() tea.Cmd { return nil }

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

func (m *Model) selected() *session.Row {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return nil
	}
	return &m.rows[m.cursor]
}

func (m *Model) selectedSession() *session.Session {
	row := m.selected()
	if row == nil || row.Node.Kind != session.KindSession {
		return nil
	}
	return row.Node.Session
}

func (m *Model) refresh() {
	m.live = map[string]tmux.SessionState{}
	if !m.client.Available() {
		return
	}
	if bySlug, err := m.client.BySlug(); err == nil {
		m.live = bySlug
	} else {
		m.errMsg = err.Error()
	}
}

func (m *Model) running() int { return len(m.live) }

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

	case attachReadyMsg:
		cmd := msg.cmd
		return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
			return attachDoneMsg{err: err}
		})

	case attachFailedMsg:
		m.errMsg = msg.err.Error()
		return m, nil

	case attachDoneMsg:
		if msg.err != nil {
			m.errMsg = "attach: " + msg.err.Error()
		} else {
			m.status = "detached — Ctrl-b d always returns to the tree"
		}
		m.refresh()
		return m, nil

	case tea.KeyMsg:
		if m.confirm != nil {
			return m.updateConfirm(msg)
		}
		if m.menu != nil {
			return m.updateMenu(msg)
		}
		if m.help {
			switch msg.String() {
			case "?", "esc", "q", "enter":
				m.help = false
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

func (m *Model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		yes := m.confirm.yes
		m.confirm = nil
		if yes != nil {
			return m, yes()
		}
	case "n", "N", "esc", "enter", "q":
		m.confirm = nil
	case "ctrl+c":
		m.confirm = nil
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) updateMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.menu = nil
	case "j", "down":
		if m.menu.index < len(m.menu.items)-1 {
			m.menu.index++
		}
	case "k", "up":
		if m.menu.index > 0 {
			m.menu.index--
		}
	case "enter":
		item := m.menu.items[m.menu.index]
		m.menu = nil
		if item.run != nil {
			return m, item.run()
		}
	case "ctrl+c":
		m.menu = nil
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
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

func (m *Model) updateNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "q":
		if m.cfg.General.ConfirmOnQuit && m.running() > 0 {
			n := m.running()
			m.confirm = &confirmState{
				prompt: fmt.Sprintf("%d session(s) still running. Quit gcrt? They keep running in tmux.", n),
				yes:    func() tea.Cmd { return tea.Quit },
			}
			return m, nil
		}
		return m, tea.Quit
	case "?":
		m.help = true
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
	case "enter":
		if s := m.selectedSession(); s != nil {
			return m, m.beginAttach(s)
		}
	case "d":
		m.openSessionMenu()
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

// beginAttach builds the argv, ensures the tmux session exists, and hands back
// the attach command for tea.ExecProcess to run. Ensure runs in a tea.Cmd so a
// slow tmux call can never block the event loop.
func (m *Model) beginAttach(s *session.Session) tea.Cmd {
	tr, err := transport.For(s, m.cfg)
	if err != nil {
		m.errMsg = err.Error()
		return nil
	}
	if err := tr.Validate(s); err != nil {
		m.errMsg = err.Error()
		return nil
	}
	argv, env, err := tr.Argv(s, "")
	if err != nil {
		m.errMsg = err.Error()
		return nil
	}

	client := m.client
	slug := s.Slug
	opts := tr.TmuxOptions(s)
	m.errMsg = ""
	m.status = "connecting…"

	return func() tea.Msg {
		if !client.Available() {
			return attachFailedMsg{err: fmt.Errorf("tmux is not installed (%s)", tmux.InstallHint())}
		}
		if err := client.Ensure(slug, argv, env, opts); err != nil {
			return attachFailedMsg{err: err}
		}
		return attachReadyMsg{cmd: client.AttachCommand(slug)}
	}
}

func (m *Model) openSessionMenu() {
	s := m.selectedSession()
	if s == nil {
		m.status = "select a session first"
		return
	}
	if _, ok := m.live[s.Slug]; !ok {
		m.status = s.Name + " is not running — enter to connect"
		return
	}

	sess := s
	m.menu = &menuState{
		title: sess.Name,
		items: []menuItem{
			{
				label: "Detach",
				hint:  "leave it running",
				run: func() tea.Cmd {
					if err := m.client.DetachSession(sess.Slug); err != nil {
						m.errMsg = err.Error()
					} else {
						m.status = sess.Name + " detached"
					}
					m.refresh()
					return nil
				},
			},
			{
				label:       "Kill",
				hint:        "stop the session and its process",
				destructive: true,
				run:         func() tea.Cmd { m.askKill(sess); return nil },
			},
		},
	}
}

func (m *Model) askKill(s *session.Session) {
	m.confirm = &confirmState{
		prompt: fmt.Sprintf("Kill %s? The session and its process stop. Logs are kept.", s.Name),
		yes: func() tea.Cmd {
			if err := m.client.Kill(s.Slug); err != nil {
				m.errMsg = err.Error()
			} else {
				m.status = s.Name + " killed"
			}
			m.refresh()
			return nil
		},
	}
}

func (m *Model) Filter() string { return m.filter }

func (m *Model) Capabilities() []string {
	caps := []string{}
	if m.client.Available() {
		caps = append(caps, "tmux ✓")
	} else {
		caps = append(caps, "tmux ✗")
	}
	caps = append(caps, "ssh "+present("ssh"))
	caps = append(caps, "vault "+m.cfg.Credentials.DefaultProvider)
	caps = append(caps, "picocom "+present("picocom"))
	return caps
}

func (m *Model) SessionCount() int { return len(m.file.Session) }

func (m *Model) Status() string { return m.status }
func (m *Model) Err() string    { return m.errMsg }

// SelectedName is the highlighted row's label, empty on an empty tree.
func (m *Model) SelectedName() string {
	row := m.selected()
	if row == nil {
		return ""
	}
	return row.Node.Label
}

func present(bin string) string {
	if _, err := exec.LookPath(bin); err != nil {
		return "✗"
	}
	return "✓"
}

func joinCaps(caps []string) string { return strings.Join(caps, "  ") }
