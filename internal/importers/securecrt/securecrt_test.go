package securecrt_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hazyumps/ghosttycrt/internal/importers/securecrt"
	"github.com/hazyumps/ghosttycrt/internal/session"
)

// fixture writes one .ini per entry into a temp Sessions directory.
func fixture(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func sshSession(host, user string, port int) string {
	return strings.Join([]string{
		`S:"Protocol Name"=SSH2`,
		`S:"Hostname"=` + host,
		`S:"Username"=` + user,
		`D:"[SSH2] Port"=` + hexDword(port),
		`S:"Firewall Name"=None`,
	}, "\n")
}

func hexDword(n int) string {
	const digits = "0123456789abcdef"
	b := make([]byte, 8)
	for i := 7; i >= 0; i-- {
		b[i] = digits[n&0xf]
		n >>= 4
	}
	return string(b)
}

func TestParseMapsHostUserPortAndGroup(t *testing.T) {
	dir := fixture(t, map[string]string{
		"Acme/dc1/edge-sw-04.ini": sshSession("192.0.2.104", "operator", 22),
		"Acme/localhost.ini":      `S:"Protocol Name"=Local Shell` + "\n",
		"Default.ini":             sshSession("ignored", "ignored", 22),
		"Acme/__FolderData__.ini": `S:"Hostname"=ignored` + "\n",
	})

	sessions, report, err := securecrt.Parse(dir)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if report.Total != 2 {
		t.Fatalf("parsed %d sessions, want 2 (folder metadata and Default.ini are not sessions)", report.Total)
	}
	if len(sessions) != 2 {
		t.Fatalf("got %d sessions, want 2", len(sessions))
	}

	byName := map[string]session.Session{}
	for _, s := range sessions {
		byName[s.Name] = s
	}

	box := byName["edge-sw-04"]
	if box.Transport != session.TransportSSH {
		t.Fatalf("transport = %q", box.Transport)
	}
	if box.Group != "Acme/dc1" {
		t.Fatalf("group = %q, want the folder path", box.Group)
	}
	if box.SSH == nil || box.SSH.Host != "192.0.2.104" || box.SSH.User != "operator" || box.SSH.Port != 22 {
		t.Fatalf("ssh block = %+v", box.SSH)
	}
	if box.Slug != "edge-sw-04" {
		t.Fatalf("slug = %q", box.Slug)
	}

	local := byName["localhost"]
	if local.Transport != session.TransportLocal {
		t.Fatalf("Local Shell should map to the local transport, got %q", local.Transport)
	}
	if local.Group != "Acme" {
		t.Fatalf("group = %q", local.Group)
	}
}

func TestParseKeepsANonDefaultPort(t *testing.T) {
	dir := fixture(t, map[string]string{"a.ini": sshSession("h", "u", 3021)})
	sessions, _, err := securecrt.Parse(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := sessions[0].SSH.Port; got != 3021 {
		t.Fatalf("port = %d, want 3021 (the value is hex in the file)", got)
	}
}

// The whole point: no password ever lands in sessions.toml, and the sessions
// that had one are named so a credential ref can be added.
func TestParseDiscardsPasswordsAndFlagsThem(t *testing.T) {
	dir := fixture(t, map[string]string{
		"saved.ini": sshSession("10.0.0.1", "root", 22) + "\n" +
			`D:"Session Password Saved"=00000001` + "\n" +
			`S:"Password V2"=02:deadbeefcafe` + "\n",
		"unsaved.ini": sshSession("10.0.0.2", "root", 22) + "\n" +
			`D:"Session Password Saved"=00000000` + "\n" +
			`S:"Password V2"=02:deadbeefcafe` + "\n",
	})

	sessions, report, err := securecrt.Parse(dir)
	if err != nil {
		t.Fatal(err)
	}

	for _, s := range sessions {
		if s.Credentials != nil {
			t.Fatalf("%s: a credentials block was written for an imported session: %+v", s.Name, s.Credentials)
		}
	}

	if len(report.NeedRef) != 1 || report.NeedRef[0] != "saved" {
		t.Fatalf("NeedRef = %v, want just the session with a saved password", report.NeedRef)
	}
}

func TestParseReportsUnsupportedProtocols(t *testing.T) {
	dir := fixture(t, map[string]string{
		"rdp.ini": `S:"Protocol Name"=RDP` + "\n" + `S:"Hostname"=10.0.0.9` + "\n",
	})
	sessions, report, err := securecrt.Parse(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Fatalf("an unsupported protocol should not import: %+v", sessions)
	}
	if len(report.Unsupported) != 1 || report.Unsupported[0] != "rdp" {
		t.Fatalf("Unsupported = %v", report.Unsupported)
	}
}

func TestParseSkipsSessionsWithNoHost(t *testing.T) {
	dir := fixture(t, map[string]string{
		"empty.ini": `S:"Protocol Name"=SSH2` + "\n" + `S:"Hostname"=` + "\n",
	})
	sessions, report, err := securecrt.Parse(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Fatalf("got %+v, want nothing", sessions)
	}
	if len(report.Skipped) != 1 || report.Skipped[0] != "empty" {
		t.Fatalf("Skipped = %v", report.Skipped)
	}
}

func TestParseIgnoresBinaryContinuationLines(t *testing.T) {
	body := sshSession("10.0.0.3", "root", 22) + "\n" +
		`B:"Mac Window Placement"=0000002c` + "\n" +
		` 2c 00 00 00 00 00 00 00 01 00 00 00 fc ff ff ff` + "\n" +
		`D:"[SSH2] Port"=00000019` + "\n"

	dir := fixture(t, map[string]string{"win.ini": body})
	sessions, _, err := securecrt.Parse(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("got %d sessions", len(sessions))
	}
	if got := sessions[0].SSH.Port; got != 25 {
		t.Fatalf("port = %d, want 25 — the key after a binary blob was lost", got)
	}
}

// Re-importing must not orphan the state and logs keyed by session id.
func TestIdsAreStableAcrossImports(t *testing.T) {
	dir := fixture(t, map[string]string{
		"a/b/one.ini": sshSession("10.0.0.1", "u", 22),
		"c/two.ini":   sshSession("10.0.0.2", "u", 22),
	})

	first, _, err := securecrt.Parse(dir)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := securecrt.Parse(dir)
	if err != nil {
		t.Fatal(err)
	}

	for i := range first {
		if first[i].ID != second[i].ID {
			t.Fatalf("%s: id changed between imports: %s then %s", first[i].Name, first[i].ID, second[i].ID)
		}
		if first[i].ID == "" {
			t.Fatalf("%s: empty id", first[i].Name)
		}
	}
}

func TestSlugsAreUniqueAndValid(t *testing.T) {
	dir := fixture(t, map[string]string{
		"one/ping 10.10.0.1.ini": sshSession("10.0.0.1", "u", 22),
		"two/ping 10.10.0.1.ini": sshSession("10.0.0.2", "u", 22),
		"tx888core01 (1).ini":    sshSession("10.0.0.3", "u", 22),
	})

	sessions, _, err := securecrt.Parse(dir)
	if err != nil {
		t.Fatal(err)
	}

	seen := map[string]bool{}
	for _, s := range sessions {
		if seen[s.Slug] {
			t.Fatalf("duplicate slug %q", s.Slug)
		}
		seen[s.Slug] = true
		if s.Slug != session.Slugify(s.Slug) || s.Slug == "" {
			t.Fatalf("slug %q is not a valid slug for %q", s.Slug, s.Name)
		}
	}
	if !seen["tx888core01-1"] {
		t.Fatalf("expected tx888core01 (1) to slugify to tx888core01-1, got %v", seen)
	}
}

func TestParseRejectsANonDirectory(t *testing.T) {
	if _, _, err := securecrt.Parse(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("expected an error for a missing directory")
	}
}
