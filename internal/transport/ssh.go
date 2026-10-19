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
	if !session.ValidTerminal(s.SSH.Terminal) {
		return errors.New("ssh: terminal must be a terminal name such as xterm, or empty to inherit")
	}
	if !session.ValidSSHAuthMethod(s.SSH.AuthMethod) {
		return errors.New("ssh: auth_method must be auto, password, or publickey")
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
	// OpenSSH keeps the first value, so a session's explicit login choice
	// precedes global extra_args and ~/.ssh/config. Some appliances disconnect
	// after a few rejected agent keys, before automatic login reaches password.
	switch sc.AuthMethod {
	case "password":
		argv = append(argv, "-o", "PubkeyAuthentication=no",
			"-o", "PreferredAuthentications=password,keyboard-interactive",
			"-o", "PasswordAuthentication=yes", "-o", "KbdInteractiveAuthentication=yes")
	case "publickey":
		argv = append(argv, "-o", "PubkeyAuthentication=yes", "-o", "PreferredAuthentications=publickey")
	}
	argv = append(argv, t.ExtraArgs...)

	target := sc.Host
	if sc.User != "" {
		target = sc.User + "@" + sc.Host
	}
	argv = append(argv, target)

	if sc.Terminal != "" {
		// tmux can overwrite TERM supplied through new-session -e. Apply it
		// after tmux starts the process so SSH's PTY request uses this value.
		// These are separate argv elements; no shell interpolation is involved.
		argv = append([]string{"env", "TERM=" + sc.Terminal}, argv...)
	}
	return argv, nil, nil
}
