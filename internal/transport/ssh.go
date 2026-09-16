package transport

import (
	"errors"
	"strconv"

	"github.com/hazyumps/ghosttycrt/internal/config"
	"github.com/hazyumps/ghosttycrt/internal/session"
)

type SSH struct {
	Binary    string
	ExtraArgs []string
}

func (SSH) Name() string { return "ssh" }

// NeedsSecret is false until M4. Today ssh authenticates through the agent, an
// identity file, or ~/.ssh/config — none of which gcrt handles.
func (SSH) NeedsSecret() bool { return false }

func (t SSH) TmuxOptions(*session.Session) map[string]string { return nil }

func (t SSH) Validate(s *session.Session) error {
	if s.SSH == nil {
		return errors.New("ssh: missing [session.ssh] block")
	}
	if s.SSH.Host == "" {
		return errors.New("ssh: host is required")
	}
	return nil
}

func (t SSH) Argv(s *session.Session, secret string) ([]string, []string, error) {
	if err := t.Validate(s); err != nil {
		return nil, nil, err
	}
	sc := s.SSH

	bin := t.Binary
	if bin == "" {
		bin = "ssh"
	}

	argv := []string{bin}
	if sc.Port != 0 && sc.Port != 22 {
		argv = append(argv, "-p", strconv.Itoa(sc.Port))
	}
	if sc.Identity != "" {
		argv = append(argv, "-i", config.ExpandPath(sc.Identity))
	}
	if sc.Jump != "" {
		argv = append(argv, "-J", sc.Jump)
	}
	argv = append(argv, t.ExtraArgs...)

	target := sc.Host
	if sc.User != "" {
		target = sc.User + "@" + sc.Host
	}
	argv = append(argv, target)

	return argv, nil, nil
}
