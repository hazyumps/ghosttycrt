package tui

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hazyumps/ghosttycrt/internal/session"
)

type quickConnect struct {
	address string
	cursor  int
	err     string
}

// parseQuickAddress accepts an SSH destination, never arbitrary command-line
// options. ssh config aliases work just like hostnames; passwords are rejected.
func parseQuickAddress(address string) (*session.SSHConfig, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return nil, fmt.Errorf("enter a hostname or user@host")
	}
	if strings.ContainsAny(address, "\r\n\t ") {
		return nil, fmt.Errorf("enter one SSH address, without spaces or command options")
	}
	if !strings.Contains(address, "://") {
		// Unbracketed IPv6 without a port is unambiguous.
		user, host := "", address
		if i := strings.LastIndexByte(address, '@'); i >= 0 {
			user, host = address[:i+1], address[i+1:]
		}
		if net.ParseIP(host) != nil && strings.Contains(host, ":") {
			address = user + "[" + host + "]"
		}
		address = "ssh://" + address
	}
	u, err := url.Parse(address)
	if err != nil {
		return nil, fmt.Errorf("use user@host, host:port, or user@[IPv6]:port")
	}
	if u.Scheme != "ssh" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.ForceQuery {
		return nil, fmt.Errorf("use an SSH address without a path, query or fragment")
	}
	user := ""
	if u.User != nil {
		if _, password := u.User.Password(); password {
			return nil, fmt.Errorf("omit the password; SSH will ask for it")
		}
		user = u.User.Username()
		if user == "" {
			return nil, fmt.Errorf("enter a username before @")
		}
	}
	host := u.Hostname()
	for _, value := range []string{host, user} {
		if strings.HasPrefix(value, "-") || strings.ContainsAny(value, "@/\\") || strings.IndexFunc(value, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
			return nil, fmt.Errorf("invalid hostname or username")
		}
	}
	if host == "" {
		return nil, fmt.Errorf("enter a hostname")
	}
	if strings.Contains(host, ":") && net.ParseIP(host) == nil {
		return nil, fmt.Errorf("invalid IPv6 address")
	}
	port := 22
	if value := u.Port(); value != "" {
		port, err = strconv.Atoi(value)
		if err != nil || port < 1 || port > 65535 {
			return nil, fmt.Errorf("port must be between 1 and 65535")
		}
	} else if strings.HasSuffix(u.Host, ":") {
		return nil, fmt.Errorf("enter a port after :")
	}
	return &session.SSHConfig{Host: host, User: user, Port: port}, nil
}

func (m *Model) openQuickConnect() tea.Cmd {
	m.quick = &quickConnect{}
	if m.workspace {
		_ = m.client.ToggleTreeZoom()
	}
	return nil
}

func (m *Model) closeQuickConnect() {
	if m.quick != nil && m.workspace {
		_ = m.client.ToggleTreeZoom()
	}
	m.quick = nil
}

func (m *Model) updateQuickConnect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	q := m.quick
	runes := []rune(q.address)
	switch msg.String() {
	case "esc", "ctrl+c":
		m.closeQuickConnect()
	case "enter":
		return m, m.connectQuick()
	case "ctrl+u":
		q.address, q.cursor = "", 0
	case "left":
		q.cursor = max(0, q.cursor-1)
	case "right":
		q.cursor = min(len(runes), q.cursor+1)
	case "home", "ctrl+a":
		q.cursor = 0
	case "end", "ctrl+e":
		q.cursor = len(runes)
	case "backspace":
		if q.cursor > 0 {
			q.address = string(runes[:q.cursor-1]) + string(runes[q.cursor:])
			q.cursor--
		}
	case "delete":
		if q.cursor < len(runes) {
			q.address = string(runes[:q.cursor]) + string(runes[q.cursor+1:])
		}
	default:
		if msg.Type == tea.KeyRunes {
			q.address = string(runes[:q.cursor]) + string(msg.Runes) + string(runes[q.cursor:])
			q.cursor += len(msg.Runes)
		}
	}
	return m, nil
}

func (m *Model) connectQuick() tea.Cmd {
	ssh, err := parseQuickAddress(m.quick.address)
	if err != nil {
		m.quick.err = err.Error()
		return nil
	}
	var target *session.Session
	for i := range m.file.Session {
		s := &m.file.Session[i]
		if s.Transport == session.TransportSSH && s.SSH != nil && strings.EqualFold(s.SSH.Host, ssh.Host) && s.SSH.User == ssh.User && orDefault(s.SSH.Port, 22) == ssh.Port {
			target = s
			break
		}
	}
	if target == nil {
		name := ssh.Host
		if strings.Contains(name, ":") {
			name = "[" + name + "]"
		}
		if ssh.Port != 22 {
			name += ":" + strconv.Itoa(ssh.Port)
		}
		if ssh.User != "" {
			name = ssh.User + "@" + name
		}
		target = &session.Session{ID: session.NewID(), Name: name, Transport: session.TransportSSH, Group: "Quick Connect", SSH: ssh}
		target.Slug = session.UniqueSlug(name, m.slugTaken(""))
		updated := m.withSession(target)
		if err := session.SaveFile(m.paths.SessionsFile(), updated); err != nil {
			m.quick.err = "could not save session: " + err.Error()
			return nil
		}
		m.file.Session = updated
	}
	m.closeQuickConnect()
	m.filter, m.filtering = "", false
	parts := target.GroupPath()
	for i := range parts {
		delete(m.collapsed, strings.Join(parts[:i+1], "/"))
	}
	m.rebuild()
	for i, row := range m.rows {
		if row.Node.Kind == session.KindSession && row.Node.Session.ID == target.ID {
			m.setCursor(i)
			break
		}
	}
	if m.workspace {
		return m.beginShow(target)
	}
	return m.beginAttach(target)
}

func (m *Model) viewQuickConnect() string {
	q := m.quick
	runes := []rune(q.address)
	input := string(runes[:q.cursor]) + "█" + string(runes[q.cursor:])
	// Keep the editing cursor visible when the address is wider than the pane.
	if q.cursor >= m.innerWidth()-2 {
		input = "…" + string(runes[max(0, q.cursor-m.innerWidth()+4):q.cursor]) + "█"
	}
	lines := []string{
		"SSH host, user@host, or user@host:port",
		"",
		styleText.Render(input),
		"",
		"New addresses are saved under Quick Connect.",
		"Existing saved sessions keep their settings.",
		"Enter: connect   Esc: cancel",
	}
	if q.err != "" {
		lines = append(lines, "", styleErr.Render(q.err))
	}
	view, _ := m.modalWithActions("Quick Connect", lines, nil, false, []button{
		{label: "Connect", run: m.connectQuick},
		{label: "Cancel", run: func() tea.Cmd { m.closeQuickConnect(); return nil }},
	})
	return view
}
