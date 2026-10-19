package tmux

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Exercise the generated shell, not a text match: unchanged idle frames must
// emit zero bytes, while resize and returning from a connection redraw once.
func TestIdleOutputOnlyChangesOnResizeOrConnection(t *testing.T) {
	dir := t.TempDir()
	write := func(name, value string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(value), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	write("size", "24 80\n")
	write("stty", "#!/bin/sh\ncat \"$IDLE_TEST_DIR/size\"\n")
	write("fake-tmux", `#!/bin/sh
case "$3" in
  has-session) test -f "$IDLE_TEST_DIR/connected" ;;
  attach) printf '<attached>'; rm "$IDLE_TEST_DIR/connected" ;;
esac
`)
	c := New("unused")
	c.Bin = filepath.Join(dir, "fake-tmux")
	write("content.sh", c.contentScript())
	logPath := filepath.Join(dir, "output")
	log, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	cmd := exec.Command("sh", filepath.Join(dir, "content.sh"))
	cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "IDLE_TEST_DIR="+dir)
	cmd.Stdout, cmd.Stderr = log, log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); _ = cmd.Wait() })
	read := func() string { data, _ := os.ReadFile(logPath); return string(data) }
	wait := func(check func(string) bool) {
		t.Helper()
		deadline := time.Now().Add(4 * time.Second)
		for time.Now().Before(deadline) {
			if check(read()) {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("idle view did not update: %q", read())
	}
	wait(func(s string) bool { return strings.Contains(s, "no connections open yet") })
	before := read()
	time.Sleep(1200 * time.Millisecond)
	if read() != before {
		t.Fatal("unchanged idle screen emitted output (would flash)")
	}
	write("size", "30 100\n")
	wait(func(s string) bool { return strings.Count(s, "\x1b[2J") == 2 })
	write("connected", "1")
	wait(func(s string) bool { return strings.Contains(s, "<attached>") && strings.Count(s, "\x1b[2J") == 3 })
	before = read()
	time.Sleep(700 * time.Millisecond)
	if read() != before {
		t.Fatal("returning to idle resumed repeated redraws")
	}
}
