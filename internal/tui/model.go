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
	offset    int
	collapsed map[string]bool

	filter    string
	filtering bool

	live map[string]tmux.SessionState

	// workspace mode: the tree is pane 0 of a tmux session and every connection
	// is a tagged pane beside it.
	workspace bool
	paneID    string
	treeWidth int
	tabs      bool
	panes     map[string]tmux.Pane

	help    bool
	helpTop int
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

type workspaceShownMsg struct {
	paneID string
	slug   string
	err    error
}

func New(cfg *config.Config, file *session.File, paths config.Paths, problems session.Problems, client *tmux.Client) *Model {
	m := &Model{
		cfg:       cfg,
		file:      file,
		paths:     paths,
		problems:  problems,
		client:    client,
		collapsed: map[string]bool{},
		live:      map[string]tmux.SessionState{},
		panes:     map[string]tmux.Pane{},
	}
	m.rebuild()
	m.cursorToFirstSession()
	m.refresh()
	return m
}

// EnableWorkspace switches the model to workspace mode: connections become
// panes or windows of the tmux session this process is already running inside.
func (m *Model) EnableWorkspace(paneID string) {
	m.workspace = true
	m.paneID = paneID
	m.treeWidth = m.cfg.General.WorkspaceTreeWidth
	m.tabs = m.cfg.General.WorkspaceLayout == config.WorkspaceTabs

	m.client.SetLayout(m.layout())
	if err := m.client.Configure(m.treeWidth, paneID); err != nil {
		m.errMsg = err.Error()
	}
	m.refresh()
}

func (m *Model) layout() tmux.Layout {
	if m.tabs {
		return tmux.LayoutTabs
	}
	return tmux.LayoutSplit
}

// paneOpen reports whether a connection is the one on screen.
func (m *Model) paneOpen(p tmux.Pane) bool { return p.Open(m.layout()) }

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
	m.clampScroll()
}

// bodyHeight is the number of rows the tree can show: everything between the
// header and the footer.
func (m *Model) bodyHeight() int {
	h := m.height - 2
	if h < 3 {
		h = 3
	}
	return h
}

func (m *Model) maxOffset() int {
	extra := len(m.rows) - m.bodyHeight()
	if extra < 0 {
		return 0
	}
	return extra
}

// clampScroll keeps the offset legal and the cursor on screen.
func (m *Model) clampScroll() {
	if m.offset > m.maxOffset() {
		m.offset = m.maxOffset()
	}
	if m.offset < 0 {
		m.offset = 0
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if bottom := m.offset + m.bodyHeight(); m.cursor >= bottom {
		m.offset = m.cursor - m.bodyHeight() + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m *Model) setCursor(i int) {
	m.cursor = i
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.clampScroll()
}

// cursorToFirstSession puts the highlight on a connectable row rather than a
// group header, which is where a user expects to land on startup.
func (m *Model) cursorToFirstSession() {
	for i, r := range m.rows {
		if r.Node.Kind == session.KindSession {
			m.setCursor(i)
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
	m.errMsg = ""
	if !m.client.Available() {
		return
	}

	if m.workspace {
		panes, err := m.client.Panes()
		if err != nil {
			m.errMsg = err.Error()
			return
		}
		m.panes = map[string]tmux.Pane{}
		for _, p := range panes {
			if p.Slug == "" {
				continue
			}
			m.panes[p.Slug] = p
		}
		return
	}

	m.live = map[string]tmux.SessionState{}
	if bySlug, err := m.client.BySlug(); err == nil {
		m.live = bySlug
	} else {
		m.errMsg = err.Error()
	}
}

// paneFor is the workspace pane backing a session, if it is running.
func (m *Model) paneFor(s *session.Session) (tmux.Pane, bool) {
	p, ok := m.panes[s.Slug]
	return p, ok
}

func (m *Model) running() int {
	if m.workspace {
		return len(m.panes)
	}
	return len(m.live)
}

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

	case workspaceShownMsg:
		if msg.err != nil {
			m.errMsg = msg.err.Error()
		} else if m.tabs {
			m.status = "switched to " + msg.slug
		} else {
			m.status = msg.slug + " is open beside the tree"
		}
		m.refresh()
		return m, nil

	case tea.MouseMsg:
		return m.updateMouse(msg)

	case tea.KeyMsg:
		// Bubbletea delivers several keystrokes as one message when they arrive
		// in a single read — a paste, or fast typing. String() would then be
		// "jj" and match no case, so replay them one at a time.
		if msg.Type == tea.KeyRunes && len(msg.Runes) > 1 {
			var last tea.Cmd
			for _, r := range msg.Runes {
				updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				m = updated.(*Model)
				if cmd != nil {
					last = cmd
				}
			}
			return m, last
		}

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
			case "j", "down":
				m.scrollHelp(1)
			case "k", "up":
				m.scrollHelp(-1)
			case "pgdown", "ctrl+f", " ":
				m.scrollHelp(m.helpWindow())
			case "pgup", "ctrl+b":
				m.scrollHelp(-m.helpWindow())
			case "g":
				m.helpTop = 0
			case "G":
				m.scrollHelp(len(m.helpContent()))
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
		return m, m.requestQuit()
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
		m.setCursor(m.cursor + 1)
	case "k", "up":
		m.setCursor(m.cursor - 1)
	case "g":
		m.setCursor(0)
	case "G":
		m.setCursor(len(m.rows) - 1)
	case "ctrl+f", "pgdown":
		m.setCursor(m.cursor + m.bodyHeight())
	case "ctrl+b", "pgup":
		m.setCursor(m.cursor - m.bodyHeight())
	case "ctrl+r", "r":
		m.refresh()
	case "right", "l":
		m.expand()
	case "left", "h":
		m.collapse()
	case "enter":
		if s := m.selectedSession(); s != nil {
			if m.workspace {
				return m, m.beginShow(s)
			}
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
			m.setCursor(i)
			return
		}
	}
}

// updateMouse makes the tree clickable: a click on a session opens it, a click
// on a group header folds it, and the wheel moves the cursor.
func (m *Model) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.confirm != nil || m.menu != nil || m.filtering {
		return m, nil
	}

	if m.help {
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.scrollHelp(-3)
		case tea.MouseButtonWheelDown:
			m.scrollHelp(3)
		}
		return m, nil
	}

	// The header row is the menu bar.
	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && msg.Y == 0 {
		_, items := m.renderBar()
		for _, item := range items {
			if msg.X >= item.start && msg.X < item.end {
				return m, item.run()
			}
		}
		return m, nil
	}

	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.setCursor(m.cursor - 3)
		return m, nil
	case tea.MouseButtonWheelDown:
		m.setCursor(m.cursor + 3)
		return m, nil
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}

	// Row 0 of the screen is the header, so the tree starts at Y == 1.
	index := m.offset + (msg.Y - 1)
	if index < 0 || index >= len(m.rows) {
		return m, nil
	}
	row := m.rows[index]
	m.setCursor(index)

	if row.Node.Kind == session.KindGroup {
		if m.collapsed[row.Node.Path] {
			delete(m.collapsed, row.Node.Path)
		} else {
			m.collapsed[row.Node.Path] = true
		}
		m.rebuild()
		return m, nil
	}

	if m.workspace {
		return m, m.beginShow(row.Node.Session)
	}
	return m, m.beginAttach(row.Node.Session)
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

// beginShow opens a session as a pane beside the tree, creating it if needed.
func (m *Model) beginShow(s *session.Session) tea.Cmd {
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
	width := m.treeWidth
	m.errMsg = ""
	m.status = "opening " + s.Name + "…"

	return func() tea.Msg {
		if !client.Available() {
			return workspaceShownMsg{err: fmt.Errorf("tmux is not installed (%s)", tmux.InstallHint())}
		}
		id, err := client.Show(slug, argv, env, width)
		return workspaceShownMsg{paneID: id, slug: slug, err: err}
	}
}

func (m *Model) detachWorkspace() tea.Cmd {
	if err := m.client.DetachSelf(); err != nil {
		m.errMsg = err.Error()
		return nil
	}
	m.status = "detached — run gcrt again to come back to it"
	return nil
}

// requestQuit is what both `q` and the menu bar's Quit item do. In workspace
// mode it detaches instead, because quitting would kill the panes that are the
// sessions.
func (m *Model) requestQuit() tea.Cmd {
	if m.workspace {
		return m.detachWorkspace()
	}
	if m.cfg.General.ConfirmOnQuit && m.running() > 0 {
		n := m.running()
		m.confirm = &confirmState{
			prompt: fmt.Sprintf("%d session(s) still running. Quit gcrt? They keep running in tmux.", n),
			yes:    func() tea.Cmd { return tea.Quit },
		}
		return nil
	}
	return tea.Quit
}

func (m *Model) openSessionMenu() {
	s := m.selectedSession()
	if s == nil {
		m.status = "select a session first"
		return
	}
	if m.workspace {
		m.openWorkspaceMenu(s)
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
				hint:        "stop it and its process",
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

func (m *Model) openWorkspaceMenu(s *session.Session) {
	pane, running := m.paneFor(s)
	items := []menuItem{}

	if !running {
		items = append(items, menuItem{
			label: "Connect",
			hint:  "nothing running yet",
			run:   func() tea.Cmd { return m.beginShow(s) },
		})
	} else {
		if m.tabs {
			if !m.paneOpen(pane) {
				items = append(items, menuItem{
					label: "Switch to it",
					hint:  "go to its tab",
					run:   func() tea.Cmd { return m.beginShow(s) },
				})
			}
		} else if pane.Visible() {
			items = append(items, menuItem{
				label: "Hide",
				hint:  "off screen, still running",
				run:   func() tea.Cmd { m.hidePane(s, pane); return nil },
			})
		} else {
			items = append(items, menuItem{
				label: "Show",
				hint:  "back beside the tree",
				run:   func() tea.Cmd { return m.beginShow(s) },
			})
		}

		items = append(items, menuItem{
			label:       "Kill",
			hint:        "stop it and its process",
			destructive: true,
			run:         func() tea.Cmd { m.askKillPane(s, pane); return nil },
		})
	}

	items = append(items, menuItem{
		label:       "Shut down workspace",
		hint:        "stop everything and exit",
		destructive: true,
		run:         func() tea.Cmd { m.askShutdownWorkspace(); return nil },
	})

	m.menu = &menuState{title: s.Name, items: items}
}

func (m *Model) hidePane(s *session.Session, pane tmux.Pane) {
	if err := m.client.Hide(pane.ID, s.Slug); err != nil {
		m.errMsg = err.Error()
	} else if err := m.client.Retile(m.treeWidth); err != nil {
		m.errMsg = err.Error()
	} else {
		m.status = s.Name + " hidden — still running"
	}
	m.refresh()
}

func (m *Model) askKillPane(s *session.Session, pane tmux.Pane) {
	m.confirm = &confirmState{
		prompt: fmt.Sprintf("Kill %s? The session and its process stop. Logs are kept.", s.Name),
		yes: func() tea.Cmd {
			if err := m.client.KillPane(pane.ID); err != nil {
				m.errMsg = err.Error()
			} else {
				m.status = s.Name + " killed"
			}
			m.refresh()
			return nil
		},
	}
}

func (m *Model) askShutdownWorkspace() {
	n := m.running()
	m.confirm = &confirmState{
		prompt: fmt.Sprintf("Shut down the workspace? %d session(s) stop and gcrt exits.", n),
		yes: func() tea.Cmd {
			if err := m.client.ShutdownWorkspace(); err != nil {
				m.errMsg = err.Error()
				return nil
			}
			return tea.Quit
		},
	}
}

// SelectedName is the highlighted row's label, empty on an empty tree.
func (m *Model) SelectedName() string {
	row := m.selected()
	if row == nil {
		return ""
	}
	return row.Node.Label
}

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

func present(bin string) string {
	if _, err := exec.LookPath(bin); err != nil {
		return "✗"
	}
	return "✓"
}

func joinCaps(caps []string) string { return strings.Join(caps, "  ") }
