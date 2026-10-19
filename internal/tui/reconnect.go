package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hazyumps/ghosttycrt/internal/session"
	"github.com/hazyumps/ghosttycrt/internal/transport"
)

func (m *Model) requestReconnect(s *session.Session) tea.Cmd {
	// Validate before interrupting anything, including edits since last connect.
	tr, err := transport.For(s, m.cfg)
	if err != nil {
		m.errMsg = err.Error()
		return nil
	}
	argv, env, err := tr.Argv(s, "")
	if err != nil {
		m.errMsg = err.Error()
		return nil
	}
	m.refresh()
	live := false
	if m.workspace {
		p, exists := m.paneFor(s)
		live = exists && !p.Dead
	} else {
		_, live = m.live[s.Slug]
	}
	client, slug, width, workspace := m.client, s.Slug, m.treeWidth, m.workspace
	start := func() tea.Cmd {
		m.status = "reconnecting " + s.Name + "…"
		return func() tea.Msg {
			if workspace {
				id, err := client.Reconnect(slug, argv, env, width, live)
				return workspaceShownMsg{paneID: id, slug: slug, err: err}
			}
			if err := client.ReconnectSession(slug, argv, env, tr.TmuxOptions(s), live); err != nil {
				return attachFailedMsg{err: err}
			}
			return attachReadyMsg{cmd: client.AttachCommand(slug)}
		}
	}
	if live {
		m.confirm = &confirmState{
			prompt: fmt.Sprintf("Reconnect %s? Its current connection and any work running there will be interrupted. Other connections stay open.", s.Name),
			yes:    start,
		}
		return nil
	}
	return start()
}
