package session

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

// Problems is the set of errors and warnings found while loading a file. Errors
// are fatal; warnings are reported but the file still loads.
type Problems struct {
	Errors   []error
	Warnings []error
}

func (p Problems) Err() error {
	if len(p.Errors) == 0 {
		return nil
	}
	return errors.Join(p.Errors...)
}

func (p Problems) OK() bool { return len(p.Errors) == 0 }

func (p Problems) Strings() []string {
	out := make([]string, 0, len(p.Errors)+len(p.Warnings))
	for _, e := range p.Errors {
		out = append(out, "error: "+e.Error())
	}
	for _, w := range p.Warnings {
		out = append(out, "warning: "+w.Error())
	}
	return out
}

// Load reads sessions.toml. A missing file is an empty tree, not an error.
func Load(path string, configuredProviders []string) (*File, Problems, error) {
	f := &File{}

	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return f, Problems{}, nil
		}
		return nil, Problems{}, err
	}

	if _, err := toml.DecodeFile(path, f); err != nil {
		return nil, Problems{}, fmt.Errorf("%s: %w", path, err)
	}

	problems := f.Validate(configuredProviders)
	return f, problems, nil
}

// Validate enforces the rules in spec/03-data-model.md.
func (f *File) Validate(configuredProviders []string) Problems {
	var p Problems

	seenID := map[string]int{}
	seenSlug := map[string]int{}

	for i := range f.Session {
		s := &f.Session[i]
		where := s.Name
		if where == "" {
			where = fmt.Sprintf("session[%d]", i)
		}

		if s.Name == "" {
			p.Errors = append(p.Errors, fmt.Errorf("%s: name is required", where))
		}
		if s.ID == "" {
			p.Errors = append(p.Errors, fmt.Errorf("%s: id is required (stable, immutable, survives renames)", where))
		} else if j, dup := seenID[s.ID]; dup {
			p.Errors = append(p.Errors, fmt.Errorf("%s: id %q is already used by session[%d]", where, s.ID, j))
		} else {
			seenID[s.ID] = i
		}

		if s.Slug == "" {
			p.Errors = append(p.Errors, fmt.Errorf("%s: slug is required", where))
		} else if !slugPattern.MatchString(s.Slug) {
			p.Errors = append(p.Errors, fmt.Errorf("%s: slug %q must match ^[a-z0-9][a-z0-9-]*$", where, s.Slug))
		} else if j, dup := seenSlug[s.Slug]; dup {
			p.Errors = append(p.Errors, fmt.Errorf("%s: slug %q is already used by session[%d]", where, s.Slug, j))
		} else {
			seenSlug[s.Slug] = i
		}

		if !containsTransport(s.Transport) {
			p.Errors = append(p.Errors, fmt.Errorf("%s: transport %q is not one of ssh, serial, telnet, local", where, s.Transport))
			continue
		}

		p.Errors = append(p.Errors, validateTransport(s, where)...)
		p.Errors = append(p.Errors, validateCredentials(s, where, configuredProviders)...)

		if s.OnConnect != nil && len(s.OnConnect.Steps) > 0 &&
			s.Transport != TransportSerial && s.Transport != TransportTelnet {
			p.Errors = append(p.Errors, fmt.Errorf("%s: on_connect is only valid for serial and telnet", where))
		}
		if s.Serial != nil {
			if _, err := os.Stat(s.Serial.Device); err != nil && s.Serial.Device != "" {
				p.Warnings = append(p.Warnings, fmt.Errorf("%s: serial device %s is not present", where, s.Serial.Device))
			}
		}
	}

	return p
}

func validateTransport(s *Session, where string) []error {
	var errs []error

	present := 0
	for _, ok := range []bool{s.SSH != nil, s.Serial != nil, s.Telnet != nil, s.Local != nil} {
		if ok {
			present++
		}
	}
	if present != 1 {
		errs = append(errs, fmt.Errorf("%s: exactly one [session.<transport>] block must be present, found %d", where, present))
		return errs
	}

	switch s.Transport {
	case TransportSSH:
		if s.SSH == nil {
			return append(errs, fmt.Errorf("%s: transport ssh requires [session.ssh]", where))
		}
		if s.SSH.Host == "" {
			errs = append(errs, fmt.Errorf("%s: ssh.host is required", where))
		}
		if s.SSH.Port < 0 || s.SSH.Port > 65535 {
			errs = append(errs, fmt.Errorf("%s: ssh.port %d is out of range", where, s.SSH.Port))
		}
	case TransportSerial:
		if s.Serial == nil {
			return append(errs, fmt.Errorf("%s: transport serial requires [session.serial]", where))
		}
		if s.Serial.Device == "" {
			errs = append(errs, fmt.Errorf("%s: serial.device is required", where))
		}
		if s.Serial.Baud != 0 && !isAllowedBaud(s.Serial.Baud) {
			errs = append(errs, fmt.Errorf("%s: serial.baud %d is not a standard rate", where, s.Serial.Baud))
		}
	case TransportTelnet:
		if s.Telnet == nil {
			return append(errs, fmt.Errorf("%s: transport telnet requires [session.telnet]", where))
		}
		if s.Telnet.Host == "" {
			errs = append(errs, fmt.Errorf("%s: telnet.host is required", where))
		}
	case TransportLocal:
		if s.Local == nil {
			return append(errs, fmt.Errorf("%s: transport local requires [session.local]", where))
		}
		if len(s.Local.Command) == 0 {
			errs = append(errs, fmt.Errorf("%s: local.command is required", where))
		}
	}
	return errs
}

func validateCredentials(s *Session, where string, configured []string) []error {
	if s.Credentials == nil {
		return nil
	}
	c := s.Credentials
	if c.Provider == "" || c.Provider == "none" {
		if c.Ref != "" {
			return []error{fmt.Errorf("%s: credentials.ref %q is set but provider is none", where, c.Ref)}
		}
		return nil
	}
	var errs []error
	found := false
	for _, name := range configured {
		if name == c.Provider {
			found = true
			break
		}
	}
	if !found {
		errs = append(errs, fmt.Errorf("%s: credential provider %q is not configured", where, c.Provider))
	}
	if c.Ref == "" {
		errs = append(errs, fmt.Errorf("%s: credentials.ref is required when provider is %s", where, c.Provider))
	} else if looksLikeLiteralSecret(c.Ref) {
		errs = append(errs, fmt.Errorf("%s: credentials.ref looks like a literal secret; store a reference, never a value", where))
	}
	return errs
}

// looksLikeLiteralSecret is the heuristic from spec/03-data-model.md rule 6: a
// long high-entropy string rather than a reference. References are shouty-snake
// or path-shaped; a secret is mixed case with digits.
func looksLikeLiteralSecret(ref string) bool {
	if len(ref) < 20 {
		return false
	}
	if strings.ContainsAny(ref, ":/ -") {
		return false
	}
	var upper, lower, digit int
	for _, r := range ref {
		switch {
		case r >= 'A' && r <= 'Z':
			upper++
		case r >= 'a' && r <= 'z':
			lower++
		case r >= '0' && r <= '9':
			digit++
		}
	}
	return upper > 0 && lower > 0 && digit > 0
}

func containsTransport(t Transport) bool {
	for _, v := range ValidTransports {
		if v == t {
			return true
		}
	}
	return false
}
