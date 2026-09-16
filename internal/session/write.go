package session

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Write emits sessions.toml in the hand-editable shape documented in
// spec/03-data-model.md: each [[session]] table followed immediately by that
// session's nested tables.
func Write(w io.Writer, sessions []Session) error {
	bw := bufio.NewWriter(w)
	fmt.Fprintln(bw, "# ghosttycrt sessions — hand-editable.")
	fmt.Fprintln(bw, "# See spec/03-data-model.md. Credentials are always provider + ref, never a value.")
	for i, s := range sessions {
		if i > 0 {
			fmt.Fprintln(bw)
		}
		writeSession(bw, s)
	}
	return bw.Flush()
}

func writeSession(bw *bufio.Writer, s Session) {
	fmt.Fprintln(bw, "[[session]]")
	kv(bw, "", "id", quote(s.ID))
	kv(bw, "", "name", quote(s.Name))
	kv(bw, "", "slug", quote(s.Slug))
	kv(bw, "", "transport", quote(string(s.Transport)))
	if s.Group != "" {
		kv(bw, "", "group", quote(s.Group))
	}
	if len(s.Tags) > 0 {
		kv(bw, "", "tags", quoteList(s.Tags))
	}
	if s.Pinned {
		kv(bw, "", "pinned", "true")
	}
	if s.Description != "" {
		kv(bw, "", "description", quote(s.Description))
	}

	if c := s.SSH; c != nil {
		fmt.Fprintln(bw, "\n  [session.ssh]")
		kv(bw, "  ", "host", quote(c.Host))
		if c.Port != 0 {
			kv(bw, "  ", "port", strconv.Itoa(c.Port))
		}
		if c.User != "" {
			kv(bw, "  ", "user", quote(c.User))
		}
		if c.Jump != "" {
			kv(bw, "  ", "jump", quote(c.Jump))
		}
		if c.Identity != "" {
			kv(bw, "  ", "identity", quote(c.Identity))
		}
	}

	if c := s.Serial; c != nil {
		fmt.Fprintln(bw, "\n  [session.serial]")
		kv(bw, "  ", "device", quote(c.Device))
		if c.Baud != 0 {
			kv(bw, "  ", "baud", strconv.Itoa(c.Baud))
		}
		if c.Databits != 0 {
			kv(bw, "  ", "databits", strconv.Itoa(c.Databits))
		}
		if c.Parity != "" {
			kv(bw, "  ", "parity", quote(c.Parity))
		}
		if c.Stopbits != 0 {
			kv(bw, "  ", "stopbits", strconv.Itoa(c.Stopbits))
		}
		if c.Flow != "" {
			kv(bw, "  ", "flow", quote(c.Flow))
		}
	}

	if c := s.Telnet; c != nil {
		fmt.Fprintln(bw, "\n  [session.telnet]")
		kv(bw, "  ", "host", quote(c.Host))
		if c.Port != 0 {
			kv(bw, "  ", "port", strconv.Itoa(c.Port))
		}
	}

	if c := s.Local; c != nil {
		fmt.Fprintln(bw, "\n  [session.local]")
		kv(bw, "  ", "command", quoteList(c.Command))
	}

	if c := s.Logging; c != nil {
		fmt.Fprintln(bw, "\n  [session.logging]")
		kv(bw, "  ", "enabled", strconv.FormatBool(c.Enabled))
	}

	if c := s.Credentials; c != nil {
		fmt.Fprintln(bw, "\n  [session.credentials]")
		kv(bw, "  ", "provider", quote(c.Provider))
		if c.Ref != "" {
			kv(bw, "  ", "ref", quote(c.Ref))
		}
	}
}

func kv(bw *bufio.Writer, indent, key, val string) {
	pad := 11 - len(key)
	if pad < 1 {
		pad = 1
	}
	fmt.Fprintf(bw, "%s%s%s= %s\n", indent, key, strings.Repeat(" ", pad), val)
}

func quote(s string) string { return strconv.Quote(s) }

func quoteList(items []string) string {
	q := make([]string, 0, len(items))
	for _, it := range items {
		q = append(q, strconv.Quote(it))
	}
	return "[" + strings.Join(q, ", ") + "]"
}
