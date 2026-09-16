// Package securecrt imports SecureCRT session files.
//
// SecureCRT keeps one .ini per session under Config/Sessions, mirroring the
// folder tree in the directory structure. Values are lines of the form
// `S:"Key"=text`, `D:"Key"=hex` or `B:"Key"=hex`, with hex continuation lines
// for binary blobs.
//
// Passwords are never imported. SecureCRT stores them encrypted, but they are
// discarded regardless: sessions that had one are reported so a credential
// reference can be added by hand.
package securecrt

import (
	"bufio"
	"crypto/sha1"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/hazyumps/ghosttycrt/internal/session"
)

// DefaultDir is where SecureCRT keeps its session files on macOS.
func DefaultDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Library", "Application Support",
		"VanDyke", "SecureCRT", "Config", "Sessions")
}

// Report says what happened, so an import is auditable before anything is
// written.
type Report struct {
	Total       int
	ByTransport map[string]int
	NeedRef     []string
	Unsupported []string
	Skipped     []string
	Notes       []string
}

func (r Report) Summary() []string {
	var out []string
	out = append(out, fmt.Sprintf("parsed %d sessions", r.Total))

	kinds := make([]string, 0, len(r.ByTransport))
	for k := range r.ByTransport {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	for _, k := range kinds {
		out = append(out, fmt.Sprintf("  %-8s %d", k, r.ByTransport[k]))
	}
	if n := len(r.Unsupported); n > 0 {
		out = append(out, fmt.Sprintf("skipped %d with an unsupported protocol: %s",
			n, strings.Join(r.Unsupported, ", ")))
	}
	if n := len(r.Skipped); n > 0 {
		out = append(out, fmt.Sprintf("skipped %d with no usable host: %s",
			n, strings.Join(r.Skipped, ", ")))
	}
	if n := len(r.NeedRef); n > 0 {
		out = append(out, fmt.Sprintf(
			"%d sessions had a saved SecureCRT password. Passwords are not imported — "+
				"add a credential ref to these:", n))
		for _, name := range r.NeedRef {
			out = append(out, "    "+name)
		}
	}
	out = append(out, r.Notes...)
	return out
}

func (r *Report) addNote(note string) {
	r.Notes = append(r.Notes, note)
}

// Parse reads every session under dir. Folder names become groups; the session
// name is the file name without .ini.
func Parse(dir string) ([]session.Session, Report, error) {
	report := Report{ByTransport: map[string]int{}}

	info, err := os.Stat(dir)
	if err != nil {
		return nil, report, fmt.Errorf("securecrt: %w", err)
	}
	if !info.IsDir() {
		return nil, report, fmt.Errorf("securecrt: %s is not a directory", dir)
	}

	var (
		out       []session.Session
		usedSlugs = map[string]int{}
	)

	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".ini") {
			return nil
		}
		// These carry folder settings and the template, not a session.
		if d.Name() == "__FolderData__.ini" || d.Name() == "Default.ini" {
			return nil
		}

		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		raw, err := parseINI(p)
		if err != nil {
			return fmt.Errorf("securecrt: %s: %w", rel, err)
		}

		name := strings.TrimSuffix(d.Name(), ".ini")
		s, ok, why := mapSession(raw, name, rel)
		if !ok {
			if why == "protocol" {
				report.Unsupported = append(report.Unsupported, name)
			} else {
				report.Skipped = append(report.Skipped, name)
			}
			return nil
		}

		s.Slug = uniqueSlug(session.Slugify(name), usedSlugs)
		out = append(out, s)
		report.Total++
		report.ByTransport[string(s.Transport)]++

		if raw.dword("Session Password Saved") == 1 || raw.dword("Use Session Password") == 1 {
			report.NeedRef = append(report.NeedRef, name)
		}
		if raw.dword("Use Login Script") == 1 {
			report.addNote(fmt.Sprintf(
				"note: %s has a SecureCRT login script. on_connect macros are not implemented yet (M4); "+
					"it will need a password on connect.", name))
		}
		return nil
	})
	if err != nil {
		return nil, report, err
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Group != out[j].Group {
			return out[i].Group < out[j].Group
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	sort.Strings(report.NeedRef)
	return out, report, nil
}

func mapSession(r raw, name, rel string) (session.Session, bool, string) {
	protocol := r.get("Protocol Name")
	host := strings.TrimSpace(r.get("Hostname"))
	user := strings.TrimSpace(r.get("Username"))

	s := session.Session{
		ID:    uuidV5(rel),
		Name:  name,
		Group: groupOf(rel),
	}

	switch protocol {
	case "SSH2":
		if host == "" {
			return s, false, "host"
		}
		s.Transport = session.TransportSSH
		cfg := &session.SSHConfig{Host: host, User: user}
		if p := r.dword("[SSH2] Port"); p > 0 {
			cfg.Port = p
		}
		if fw := strings.TrimSpace(r.get("Firewall Name")); fw != "" && !strings.EqualFold(fw, "none") {
			cfg.Jump = fw
		}
		s.SSH = cfg

	case "Serial":
		device := r.get("Serial Port")
		if device == "" {
			return s, false, "host"
		}
		s.Transport = session.TransportSerial
		s.Serial = &session.SerialConfig{
			Device:   device,
			Baud:     r.dword("Baud Rate"),
			Databits: r.dword("Data Bits"),
			Parity:   strings.ToLower(r.get("Parity")),
			Stopbits: r.dword("Stop Bits"),
			Flow:     strings.ToLower(r.get("Flow Control")),
		}

	case "Telnet":
		if host == "" {
			return s, false, "host"
		}
		s.Transport = session.TransportTelnet
		cfg := &session.TelnetConfig{Host: host}
		if p := r.dword("[Telnet] Port"); p > 0 {
			cfg.Port = p
		}
		s.Telnet = cfg

	case "Local Shell", "Local":
		s.Transport = session.TransportLocal
		s.Local = &session.LocalConfig{Command: []string{"$SHELL"}}

	default:
		return s, false, "protocol"
	}

	return s, true, ""
}

// groupOf turns the session's folder into a "/"-delimited group.
func groupOf(rel string) string {
	dir := filepath.ToSlash(filepath.Dir(rel))
	dir = strings.Trim(dir, "./")
	if dir == "." {
		return ""
	}
	return dir
}

func uniqueSlug(slug string, used map[string]int) string {
	if slug == "" {
		slug = "session"
	}
	used[slug]++
	if used[slug] == 1 {
		return slug
	}
	for {
		candidate := fmt.Sprintf("%s-%d", slug, used[slug])
		if used[candidate] == 0 {
			used[candidate]++
			used[slug]++
			return candidate
		}
		used[slug]++
	}
}

// raw is one session file: option name -> value as written.
type raw map[string]string

var lineRE = regexp.MustCompile(`^([SDB]):"((?:[^"\\]|\\.)*)"=(.*)$`)

func parseINI(path string) (raw, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := raw{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		m := lineRE.FindStringSubmatch(line)
		if m == nil {
			// A continuation of a binary value, or a stray line.
			continue
		}
		out[m[2]] = m[3]
	}
	return out, sc.Err()
}

func (m raw) get(key string) string { return m[key] }

func (m raw) dword(key string) int {
	v := strings.TrimSpace(m[key])
	if v == "" {
		return 0
	}
	n, err := strconv.ParseUint(v, 16, 32)
	if err != nil {
		return 0
	}
	return int(n)
}

// namespace is the RFC 4122 DNS namespace. Ids are a v5 hash of the session's
// path relative to the Sessions directory, so re-importing the same config
// yields the same ids and never orphans state or logs.
var namespace = [16]byte{
	0x6b, 0xa7, 0xb8, 0x10, 0x9d, 0xad, 0x11, 0xd1,
	0x80, 0xb4, 0x00, 0xc0, 0x4f, 0xd4, 0x30, 0xc8,
}

func uuidV5(name string) string {
	h := sha1.New()
	h.Write(namespace[:])
	io.WriteString(h, name)
	sum := h.Sum(nil)

	var u [16]byte
	copy(u[:], sum[:16])
	u[6] = (u[6] & 0x0f) | 0x50
	u[8] = (u[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", u[0:4], u[4:6], u[6:8], u[8:10], u[10:16])
}
