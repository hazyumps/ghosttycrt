package transport

import (
	"fmt"

	"github.com/hazyumps/ghosttycrt/internal/config"
	"github.com/hazyumps/ghosttycrt/internal/session"
)

// Transport turns a session record into a tmux session. That is its only job:
// (Session) -> argv, plus whatever environment and tmux options the connection
// needs.
type Transport interface {
	Name() string

	// Argv returns the command to run in the tmux pane and any environment it
	// needs. A secret is passed via env or a pipe, never as an argv element,
	// because argv is world-readable in ps for the life of the process.
	Argv(s *session.Session, secret string) (argv []string, env []string, err error)

	NeedsSecret() bool

	// TmuxOptions are set-option pairs applied to the session after creation.
	TmuxOptions(s *session.Session) map[string]string

	Validate(s *session.Session) error
}

// ErrNotImplemented marks a transport the spec has scheduled for a later
// milestone, so the UI can say so plainly instead of failing obscurely.
type ErrNotImplemented struct {
	Name      string
	Milestone string
}

func (e ErrNotImplemented) Error() string {
	return fmt.Sprintf("transport %q lands in %s", e.Name, e.Milestone)
}

// For selects the transport for a session.
func For(s *session.Session, cfg *config.Config) (Transport, error) {
	switch s.Transport {
	case session.TransportSSH:
		return SSH{ExtraArgs: cfg.Transports.SSH.ExtraArgs, Binary: cfg.Transports.SSH.Binary}, nil
	case session.TransportSerial:
		return nil, ErrNotImplemented{Name: "serial", Milestone: "M2"}
	case session.TransportTelnet:
		return nil, ErrNotImplemented{Name: "telnet", Milestone: "M2"}
	case session.TransportLocal:
		return nil, ErrNotImplemented{Name: "local", Milestone: "M2"}
	default:
		return nil, fmt.Errorf("unknown transport %q", s.Transport)
	}
}
