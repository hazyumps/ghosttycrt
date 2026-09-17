package session

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
)

// SaveFile writes sessions.toml atomically: a temp file in the same directory,
// then a rename. A crash mid-write leaves the previous file untouched, which
// matters because this is the only copy of the session tree.
func SaveFile(path string, sessions []Session) error {
	var buf bytes.Buffer
	if err := Write(&buf, sessions); err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".sessions-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer func() {
		if err != nil {
			os.Remove(name)
		}
	}()

	if _, err = tmp.Write(buf.Bytes()); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}

	err = os.Rename(name, path)
	return err
}

// NewID mints a UUIDv4 for a session created in the UI. Ids are immutable, so
// renames never orphan state or logs.
func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand does not fail in practice; a zero id would collide.
		panic("session: cannot read random bytes: " + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// UniqueSlug derives a slug from name that no other session is using.
func UniqueSlug(name string, taken func(string) bool) string {
	base := Slugify(name)
	if base == "" {
		base = "session"
	}
	if !taken(base) {
		return base
	}
	for n := 2; ; n++ {
		candidate := fmt.Sprintf("%s-%d", base, n)
		if !taken(candidate) {
			return candidate
		}
	}
}
