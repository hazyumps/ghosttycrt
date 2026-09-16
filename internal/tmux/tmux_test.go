package tmux_test

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/hazyumps/ghosttycrt/internal/tmux"
)

// Each test run gets its own socket, so tests never touch real sessions.
func testClient(t *testing.T) *tmux.Client {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	c := tmux.New(fmt.Sprintf("gcrt-test-%d", os.Getpid()))
	t.Cleanup(func() {
		exec.Command("tmux", "-L", c.Socket, "kill-server").Run()
	})
	return c
}

func TestListWithNoServerIsEmptyNotAnError(t *testing.T) {
	c := testClient(t)
	states, err := c.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(states) != 0 {
		t.Fatalf("states = %v, want none", states)
	}
}

func TestEnsureCreatesThenReusesTheSession(t *testing.T) {
	c := testClient(t)

	if err := c.Ensure("k3s-01", []string{"sleep", "60"}, nil, nil); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if !c.Has("k3s-01") {
		t.Fatal("Has = false after Ensure")
	}

	before, err := c.BySlug()
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 1 {
		t.Fatalf("BySlug = %v, want one session", before)
	}

	// AC-8: enter on a running session returns to it rather than recreating it.
	time.Sleep(1100 * time.Millisecond)
	if err := c.Ensure("k3s-01", []string{"sleep", "60"}, nil, nil); err != nil {
		t.Fatalf("second Ensure: %v", err)
	}
	after, err := c.BySlug()
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 {
		t.Fatalf("BySlug after second Ensure = %v, want one session", after)
	}
	if before["k3s-01"].Created != after["k3s-01"].Created {
		t.Fatalf("session was recreated: created %d -> %d",
			before["k3s-01"].Created, after["k3s-01"].Created)
	}
}

func TestBySlugIgnoresForeignSessions(t *testing.T) {
	c := testClient(t)

	if err := c.Ensure("ours", []string{"sleep", "60"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("tmux", "-L", c.Socket, "new-session", "-d", "-s", "someone-else", "sleep", "60").CombinedOutput(); err != nil {
		t.Fatalf("creating foreign session: %v: %s", err, out)
	}

	bySlug, err := c.BySlug()
	if err != nil {
		t.Fatal(err)
	}
	if len(bySlug) != 1 {
		t.Fatalf("BySlug = %v, want only the gscrt/ session", bySlug)
	}
	if _, ok := bySlug["ours"]; !ok {
		t.Fatalf("BySlug = %v, want key %q", bySlug, "ours")
	}
}

func TestEnsureAppliesTmuxOptions(t *testing.T) {
	c := testClient(t)

	if err := c.Ensure("console", []string{"sleep", "60"}, nil, map[string]string{"remain-on-exit": "on"}); err != nil {
		t.Fatal(err)
	}

	out, err := exec.Command("tmux", "-L", c.Socket, "show-options", "-t", tmux.SessionName("console"), "remain-on-exit").Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := string(out); got == "" || got[:len("remain-on-exit on")] != "remain-on-exit on" {
		t.Fatalf("remain-on-exit = %q, want on", got)
	}
}

func TestKillRemovesTheSession(t *testing.T) {
	c := testClient(t)

	if err := c.Ensure("k3s-01", []string{"sleep", "60"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := c.Kill("k3s-01"); err != nil {
		t.Fatalf("Kill: %v", err)
	}
	if c.Has("k3s-01") {
		t.Fatal("session survived Kill")
	}
}

func TestEnvIsPassedIntoThePane(t *testing.T) {
	c := testClient(t)

	argv := []string{"sh", "-c", "printf '%s' \"$GCRT_TEST\" > /tmp/gcrt-env-" + c.Socket}
	if err := c.Ensure("env", argv, []string{"GCRT_TEST=passed"}, nil); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	path := "/tmp/gcrt-env-" + c.Socket
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(path); err == nil {
			if string(b) != "passed" {
				t.Fatalf("pane env = %q, want passed", b)
			}
			os.Remove(path)
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("pane never wrote the env marker")
}
