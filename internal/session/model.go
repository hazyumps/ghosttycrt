package session

import (
	"fmt"
	"regexp"
	"strings"
)

type Transport string

const (
	TransportSSH    Transport = "ssh"
	TransportSerial Transport = "serial"
	TransportTelnet Transport = "telnet"
	TransportLocal  Transport = "local"
)

var ValidTransports = []Transport{
	TransportSSH, TransportSerial, TransportTelnet, TransportLocal,
}

// File is the on-disk shape of sessions.toml: one [[session]] table per host.
type File struct {
	Session []Session `toml:"session"`
}

type Session struct {
	ID          string    `toml:"id"`
	Name        string    `toml:"name"`
	Slug        string    `toml:"slug"`
	Transport   Transport `toml:"transport"`
	Group       string    `toml:"group"`
	Tags        []string  `toml:"tags"`
	Pinned      bool      `toml:"pinned"`
	Description string    `toml:"description"`

	SSH    *SSHConfig    `toml:"ssh"`
	Serial *SerialConfig `toml:"serial"`
	Telnet *TelnetConfig `toml:"telnet"`
	Local  *LocalConfig  `toml:"local"`

	Logging     *LoggingConfig     `toml:"logging"`
	Credentials *CredentialsConfig `toml:"credentials"`
	OnConnect   *OnConnectConfig   `toml:"on_connect"`
}

type SSHConfig struct {
	Host     string `toml:"host"`
	Port     int    `toml:"port"`
	User     string `toml:"user"`
	Jump     string `toml:"jump"`
	Identity string `toml:"identity"`
}

type SerialConfig struct {
	Device   string `toml:"device"`
	Baud     int    `toml:"baud"`
	Databits int    `toml:"databits"`
	Parity   string `toml:"parity"`
	Stopbits int    `toml:"stopbits"`
	Flow     string `toml:"flow"`
}

type TelnetConfig struct {
	Host string `toml:"host"`
	Port int    `toml:"port"`
}

type LocalConfig struct {
	Command []string `toml:"command"`
}

type LoggingConfig struct {
	Enabled bool `toml:"enabled"`
}

// CredentialsConfig is always a provider plus a reference. There is no field
// anywhere in sessions.toml that holds a secret.
type CredentialsConfig struct {
	Provider string `toml:"provider"`
	Ref      string `toml:"ref"`
}

type OnConnectConfig struct {
	Steps []OnConnectStep `toml:"steps"`
}

type OnConnectStep struct {
	Expect string `toml:"expect"`
	Send   string `toml:"send"`
	Redact bool   `toml:"redact"`
}

var (
	slugPattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	allowedBauds = []int{300, 1200, 2400, 4800, 9600, 19200, 38400, 57600, 115200, 230400}
)

// Slugify derives a tmux-safe slug from a display name.
func Slugify(name string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// GroupPath splits a "/"-delimited group into its elements, dropping empties.
func (s Session) GroupPath() []string {
	parts := strings.Split(s.Group, "/")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Endpoint is the human-readable address shown in the details pane.
func (s Session) Endpoint() string {
	switch s.Transport {
	case TransportSSH:
		if s.SSH == nil {
			return ""
		}
		host := s.SSH.Host
		if s.SSH.Port != 0 && s.SSH.Port != 22 {
			host = fmt.Sprintf("%s:%d", host, s.SSH.Port)
		}
		if s.SSH.User != "" {
			return s.SSH.User + "@" + host
		}
		return host
	case TransportSerial:
		if s.Serial == nil {
			return ""
		}
		return s.Serial.Device
	case TransportTelnet:
		if s.Telnet == nil {
			return ""
		}
		if s.Telnet.Port != 0 && s.Telnet.Port != 23 {
			return fmt.Sprintf("%s:%d", s.Telnet.Host, s.Telnet.Port)
		}
		return s.Telnet.Host
	case TransportLocal:
		if s.Local == nil {
			return ""
		}
		return strings.Join(s.Local.Command, " ")
	}
	return ""
}

// CredentialRef renders provider:ref, or "none".
func (s Session) CredentialRef() string {
	if s.Credentials == nil || s.Credentials.Provider == "" || s.Credentials.Provider == "none" {
		return "none"
	}
	if s.Credentials.Ref == "" {
		return s.Credentials.Provider
	}
	return s.Credentials.Provider + ":" + s.Credentials.Ref
}

func (s Session) LoggingEnabled(def bool) bool {
	if s.Logging == nil {
		return def
	}
	return s.Logging.Enabled
}

func isAllowedBaud(b int) bool {
	for _, a := range allowedBauds {
		if a == b {
			return true
		}
	}
	return false
}
