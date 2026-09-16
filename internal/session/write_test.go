package session

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteRoundTripsThroughLoad(t *testing.T) {
	in := []Session{
		{
			ID: "id-1", Name: "core-sw-01", Slug: "core-sw-01", Transport: TransportSSH,
			Group: "network/switches", Tags: []string{"cisco", "prod"}, Pinned: true,
			Description: "Cisco C9300, rack A",
			SSH:         &SSHConfig{Host: "10.1.3.11", Port: 2222, User: "admin", Jump: "bastion"},
			Logging:     &LoggingConfig{Enabled: true},
			Credentials: &CredentialsConfig{Provider: "infisical", Ref: "CORE-SW-01-PASS"},
		},
		{
			ID: "id-2", Name: "console", Slug: "console", Transport: TransportSerial,
			Serial: &SerialConfig{Device: "/dev/tty.usbserial-1420", Baud: 9600, Databits: 8, Parity: "none", Stopbits: 1, Flow: "none"},
		},
	}

	var buf bytes.Buffer
	if err := Write(&buf, in); err != nil {
		t.Fatalf("Write: %v", err)
	}

	path := filepath.Join(t.TempDir(), "sessions.toml")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	out, problems, err := Load(path, []string{"infisical"})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !problems.OK() {
		t.Fatalf("round trip did not validate: %v", problems.Strings())
	}
	if len(out.Session) != len(in) {
		t.Fatalf("got %d sessions, want %d", len(out.Session), len(in))
	}

	got := out.Session[0]
	if got.Name != in[0].Name || got.Group != in[0].Group || !got.Pinned || got.Description != in[0].Description {
		t.Fatalf("top-level fields did not round trip: %+v", got)
	}
	if got.SSH == nil || got.SSH.Host != "10.1.3.11" || got.SSH.Port != 2222 || got.SSH.Jump != "bastion" {
		t.Fatalf("ssh block did not round trip: %+v", got.SSH)
	}
	if got.Credentials == nil || got.Credentials.Ref != "CORE-SW-01-PASS" {
		t.Fatalf("credentials did not round trip: %+v", got.Credentials)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "cisco" {
		t.Fatalf("tags did not round trip: %v", got.Tags)
	}
	if s := out.Session[1]; s.Serial == nil || s.Serial.Baud != 9600 || s.Serial.Device != "/dev/tty.usbserial-1420" {
		t.Fatalf("serial block did not round trip: %+v", s.Serial)
	}
}

func TestWriteOmitsEmptyOptionalFields(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, []Session{{
		ID: "1", Name: "a", Slug: "a", Transport: TransportSSH,
		SSH: &SSHConfig{Host: "h"},
	}}); err != nil {
		t.Fatal(err)
	}

	keys := map[string]bool{}
	for _, line := range strings.Split(buf.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}
		if k, _, ok := strings.Cut(line, "="); ok {
			keys[strings.TrimSpace(k)] = true
		}
	}

	for _, unwanted := range []string{"group", "tags", "description", "port", "user", "jump", "identity"} {
		if keys[unwanted] {
			t.Errorf("output should not set %q when it is unset:\n%s", unwanted, buf.String())
		}
	}
	for _, wanted := range []string{"id", "name", "slug", "transport", "host"} {
		if !keys[wanted] {
			t.Errorf("output should set %q:\n%s", wanted, buf.String())
		}
	}
}
