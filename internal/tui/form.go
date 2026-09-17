package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hazyumps/ghosttycrt/internal/config"
	"github.com/hazyumps/ghosttycrt/internal/session"
)

// The session form: one screen, a list of fields, no wizard. Editing happens in
// place — enter to type into a field, space to cycle an enum — and ctrl+s saves
// the whole file atomically.

type fieldKind int

const (
	fieldText fieldKind = iota
	fieldNumber
	fieldCycle
)

type formSpec struct {
	key     string
	section string
	label   string
	kind    fieldKind
	choices []string
}

type form struct {
	specs  []formSpec
	values map[string]string
	index  int

	editing bool
	buffer  string

	err string

	// identity of what is being edited
	id       string
	original *session.Session // nil for a new session
	isNew    bool

	transportChoices []string
	providerChoices  []string
}

var (
	transportChoices = []string{"ssh", "serial", "telnet", "local"}
	parityChoices    = []string{"none", "even", "odd"}
	flowChoices      = []string{"none", "software", "hardware"}
	onOff            = []string{"on", "off"}
)

func onOffValue(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

// newForm builds the field list for a session, or a blank one when s is nil.
func newForm(cfg *config.Config, s *session.Session, isNew bool) *form {
	f := &form{
		values: map[string]string{},
		isNew:  isNew,
	}
	f.transportChoices = transportChoices
	f.providerChoices = append([]string{"none"}, providerNames(cfg)...)

	if s != nil {
		f.id = s.ID
		copy := *s
		f.original = &copy
		f.load(s)
	} else {
		f.values["transport"] = cfg.General.DefaultTransport
		f.values["pinned"] = "off"
		f.values["logging"] = onOffValue(cfg.Logging.EnabledByDefault)
		f.values["provider"] = "none"
		f.values["port"] = "22"
		f.values["baud"] = "9600"
		f.values["databits"] = "8"
		f.values["parity"] = "none"
		f.values["stopbits"] = "1"
		f.values["flow"] = "none"
		f.values["telnet_port"] = "23"
	}
	f.rebuild()
	return f
}

func providerNames(cfg *config.Config) []string {
	names := make([]string, 0, len(cfg.Credentials.Providers))
	for name := range cfg.Credentials.Providers {
		names = append(names, name)
	}
	// Deterministic order for cycling.
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && names[j] < names[j-1]; j-- {
			names[j], names[j-1] = names[j-1], names[j]
		}
	}
	return names
}

func (f *form) load(s *session.Session) {
	f.values["name"] = s.Name
	f.values["group"] = s.Group
	f.values["tags"] = strings.Join(s.Tags, ", ")
	f.values["description"] = s.Description
	f.values["pinned"] = onOffValue(s.Pinned)
	f.values["transport"] = string(s.Transport)
	f.values["logging"] = onOffValue(s.LoggingEnabled(false))

	if c := s.SSH; c != nil {
		f.values["host"] = c.Host
		f.values["port"] = strconv.Itoa(orDefault(c.Port, 22))
		f.values["user"] = c.User
		f.values["jump"] = c.Jump
		f.values["identity"] = c.Identity
	}
	if c := s.Serial; c != nil {
		f.values["device"] = c.Device
		f.values["baud"] = strconv.Itoa(orDefault(c.Baud, 9600))
		f.values["databits"] = strconv.Itoa(orDefault(c.Databits, 8))
		f.values["parity"] = orDefault(c.Parity, "none")
		f.values["stopbits"] = strconv.Itoa(orDefault(c.Stopbits, 1))
		f.values["flow"] = orDefault(c.Flow, "none")
	}
	if c := s.Telnet; c != nil {
		f.values["telnet_host"] = c.Host
		f.values["telnet_port"] = strconv.Itoa(orDefault(c.Port, 23))
	}
	if c := s.Local; c != nil {
		f.values["command"] = strings.Join(c.Command, " ")
	}
	if c := s.Credentials; c != nil {
		f.values["provider"] = orDefault(c.Provider, "none")
		f.values["ref"] = c.Ref
	} else {
		f.values["provider"] = "none"
	}
}

func orDefault[T comparable](v, def T) T {
	var zero T
	if v == zero {
		return def
	}
	return v
}

// rebuild recomputes the visible fields from the current transport, keeping
// whatever has already been typed.
func (f *form) rebuild() {
	transport := f.values["transport"]

	specs := []formSpec{
		{key: "name", label: "name", kind: fieldText},
		{key: "group", label: "group", kind: fieldText},
		{key: "tags", label: "tags", kind: fieldText},
		{key: "description", label: "description", kind: fieldText},
		{key: "pinned", label: "pinned", kind: fieldCycle, choices: onOff},
		{key: "transport", section: "transport", label: "kind", kind: fieldCycle, choices: f.transportChoices},
	}

	switch session.Transport(transport) {
	case session.TransportSerial:
		specs = append(specs,
			formSpec{key: "device", label: "device", kind: fieldText},
			formSpec{key: "baud", label: "baud", kind: fieldNumber},
			formSpec{key: "databits", label: "databits", kind: fieldNumber},
			formSpec{key: "parity", label: "parity", kind: fieldCycle, choices: parityChoices},
			formSpec{key: "stopbits", label: "stopbits", kind: fieldNumber},
			formSpec{key: "flow", label: "flow", kind: fieldCycle, choices: flowChoices},
		)
	case session.TransportTelnet:
		specs = append(specs,
			formSpec{key: "telnet_host", label: "host", kind: fieldText},
			formSpec{key: "telnet_port", label: "port", kind: fieldNumber},
		)
	case session.TransportLocal:
		specs = append(specs, formSpec{key: "command", label: "command", kind: fieldText})
	default:
		specs = append(specs,
			formSpec{key: "host", label: "host", kind: fieldText},
			formSpec{key: "port", label: "port", kind: fieldNumber},
			formSpec{key: "user", label: "user", kind: fieldText},
			formSpec{key: "jump", label: "jump", kind: fieldText},
			formSpec{key: "identity", label: "identity", kind: fieldText},
		)
	}

	specs = append(specs,
		formSpec{key: "logging", section: "logging", label: "enabled", kind: fieldCycle, choices: onOff},
		formSpec{key: "provider", section: "credentials", label: "provider", kind: fieldCycle, choices: f.providerChoices},
		formSpec{key: "ref", label: "ref", kind: fieldText},
	)

	f.specs = specs
	if f.index >= len(specs) {
		f.index = len(specs) - 1
	}
	if f.index < 0 {
		f.index = 0
	}
}

func (f *form) current() formSpec { return f.specs[f.index] }

func (f *form) value(key string) string { return f.values[key] }

// set writes a field and rebuilds, so changing the transport immediately
// reveals that transport's fields.
func (f *form) set(key, value string) {
	f.values[key] = value
	if key == "transport" {
		f.rebuild()
	}
}

func (f *form) cycle(delta int) {
	spec := f.current()
	if spec.kind != fieldCycle || len(spec.choices) == 0 {
		return
	}
	cur := 0
	for i, c := range spec.choices {
		if c == f.values[spec.key] {
			cur = i
			break
		}
	}
	next := (cur + delta + len(spec.choices)) % len(spec.choices)
	f.set(spec.key, spec.choices[next])
}

func (f *form) move(delta int) {
	if f.editing {
		return
	}
	f.index = (f.index + delta + len(f.specs)) % len(f.specs)
}

func (f *form) startEdit() {
	spec := f.current()
	if spec.kind == fieldCycle {
		f.cycle(1)
		return
	}
	f.editing = true
	f.buffer = f.values[spec.key]
}

func (f *form) commitEdit() {
	if !f.editing {
		return
	}
	spec := f.current()
	value := f.buffer
	if spec.kind == fieldNumber {
		if value != "" {
			if _, err := strconv.Atoi(value); err != nil {
				f.err = fmt.Sprintf("%s must be a number", spec.label)
				return
			}
		}
	}
	f.err = ""
	f.set(spec.key, value)
	f.editing = false
	f.buffer = ""
}

// update handles a key while the form is open. handled is false for keys the
// form does not use, so the caller can ignore them rather than act on them.
func (f *form) update(msg tea.KeyMsg) (save bool, close bool, handled bool) {
	key := msg.String()

	if f.editing {
		switch key {
		case "esc":
			f.editing = false
			f.buffer = ""
		case "enter":
			f.commitEdit()
		case "backspace":
			if r := []rune(f.buffer); len(r) > 0 {
				f.buffer = string(r[:len(r)-1])
			}
		case "ctrl+u":
			f.buffer = ""
		default:
			if msg.Type == tea.KeyRunes {
				f.buffer += string(msg.Runes)
			} else if key == " " {
				f.buffer += " "
			}
		}
		return false, false, true
	}

	switch key {
	case "esc":
		return false, true, true
	case "ctrl+s":
		return true, false, true
	// Arrows and tab only: a form that eats letters as navigation cannot be
	// typed into (a session called "k3s-01" would lose its first character).
	case "up", "shift+tab":
		f.move(-1)
	case "down", "tab":
		f.move(1)
	case "enter", " ":
		f.startEdit()
	case "left":
		f.cycle(-1)
	case "right":
		f.cycle(1)
	case "ctrl+d":
		f.set(f.current().key, "")
	default:
		if msg.Type == tea.KeyRunes {
			f.startEdit()
			f.buffer += string(msg.Runes)
		}
	}
	return false, false, true
}

// toSession turns the fields back into a session, with a slug that does not
// collide and an id that survives a rename.
func (f *form) toSession(taken func(string) bool) *session.Session {
	s := &session.Session{
		ID:          f.id,
		Name:        strings.TrimSpace(f.value("name")),
		Group:       strings.TrimSpace(f.value("group")),
		Description: strings.TrimSpace(f.value("description")),
		Pinned:      f.value("pinned") == "on",
		Transport:   session.Transport(f.value("transport")),
	}
	if s.ID == "" {
		s.ID = session.NewID()
	}
	s.Slug = session.UniqueSlug(s.Name, taken)

	if tags := strings.TrimSpace(f.value("tags")); tags != "" {
		for _, t := range strings.Split(tags, ",") {
			if t = strings.TrimSpace(t); t != "" {
				s.Tags = append(s.Tags, t)
			}
		}
	}

	atoi := func(key string) int {
		n, _ := strconv.Atoi(f.value(key))
		return n
	}

	switch s.Transport {
	case session.TransportSSH:
		s.SSH = &session.SSHConfig{
			Host:     strings.TrimSpace(f.value("host")),
			Port:     atoi("port"),
			User:     strings.TrimSpace(f.value("user")),
			Jump:     strings.TrimSpace(f.value("jump")),
			Identity: strings.TrimSpace(f.value("identity")),
		}
	case session.TransportSerial:
		s.Serial = &session.SerialConfig{
			Device:   strings.TrimSpace(f.value("device")),
			Baud:     atoi("baud"),
			Databits: atoi("databits"),
			Parity:   f.value("parity"),
			Stopbits: atoi("stopbits"),
			Flow:     f.value("flow"),
		}
	case session.TransportTelnet:
		s.Telnet = &session.TelnetConfig{
			Host: strings.TrimSpace(f.value("telnet_host")),
			Port: atoi("telnet_port"),
		}
	case session.TransportLocal:
		fields := strings.Fields(f.value("command"))
		if len(fields) > 0 {
			s.Local = &session.LocalConfig{Command: fields}
		} else {
			s.Local = &session.LocalConfig{Command: []string{"$SHELL"}}
		}
	}

	s.Logging = &session.LoggingConfig{Enabled: f.value("logging") == "on"}

	if provider := f.value("provider"); provider != "" && provider != "none" {
		s.Credentials = &session.CredentialsConfig{
			Provider: provider,
			Ref:      strings.TrimSpace(f.value("ref")),
		}
	}
	return s
}
