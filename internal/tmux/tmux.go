package tmux

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// Socket is the private tmux socket gcrt uses. Keeping our sessions off the
// default socket means gcrt never collides with a user's own tmux server, and
// tests can use their own.
const Socket = "gcrt"

// Prefix namespaces gcrt sessions inside the socket, so gcrt only ever touches
// sessions it created.
const Prefix = "gscrt/"

func SessionName(slug string) string { return Prefix + slug }

type Client struct {
	Socket string
	Bin    string
	Layout Layout
}

func New(socket string) *Client { return &Client{Socket: socket, Bin: "tmux"} }

func (c *Client) args(rest ...string) []string {
	return append([]string{"-L", c.Socket}, rest...)
}

func (c *Client) Available() bool {
	_, err := exec.LookPath(c.Bin)
	return err == nil
}

func (c *Client) run(rest ...string) (string, error) {
	cmd := exec.Command(c.Bin, c.args(rest...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return stdout.String(), fmt.Errorf("tmux %s: %s", strings.Join(rest, " "), detail)
	}
	return stdout.String(), nil
}

type SessionState struct {
	Name     string
	Attached int
	Windows  int
	Created  int64
	Activity int64
}

func noServer(msg string) bool {
	return strings.Contains(msg, "no server running") ||
		strings.Contains(msg, "no sessions") ||
		strings.Contains(msg, "error connecting to")
}

func (c *Client) List() ([]SessionState, error) {
	out, err := c.run("list-sessions", "-F",
		"#{session_name}\t#{session_attached}\t#{session_windows}\t#{session_created}\t#{session_activity}")
	if err != nil {
		if noServer(err.Error()) {
			return nil, nil
		}
		return nil, err
	}

	var states []SessionState
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 5 {
			continue
		}
		st := SessionState{Name: f[0]}
		st.Attached, _ = strconv.Atoi(f[1])
		st.Windows, _ = strconv.Atoi(f[2])
		st.Created, _ = strconv.ParseInt(f[3], 10, 64)
		st.Activity, _ = strconv.ParseInt(f[4], 10, 64)
		states = append(states, st)
	}
	return states, nil
}

// BySlug indexes live gcrt sessions by slug.
func (c *Client) BySlug() (map[string]SessionState, error) {
	states, err := c.List()
	if err != nil {
		return nil, err
	}
	bySlug := map[string]SessionState{}
	for _, s := range states {
		if slug, ok := strings.CutPrefix(s.Name, Prefix); ok {
			bySlug[slug] = s
		}
	}
	return bySlug, nil
}

func (c *Client) Has(slug string) bool {
	return c.HasTarget(SessionName(slug))
}

// Ensure creates the session if it is absent and applies opts either way, so
// attach is always idempotent: enter on a running session goes back to it.
func (c *Client) Ensure(slug string, argv, env []string, opts map[string]string) error {
	name := SessionName(slug)

	if !c.Has(slug) {
		args := []string{"new-session", "-d", "-s", name}
		if cwd := startDir(); cwd != "" {
			args = append(args, "-c", cwd)
		}
		for _, e := range env {
			args = append(args, "-e", e)
		}
		args = append(args, argv...)
		if _, err := c.run(args...); err != nil {
			return err
		}
	}

	for k, v := range opts {
		if _, err := c.run("set-option", "-t", name, k, v); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) AttachCommand(slug string) *exec.Cmd {
	return exec.Command(c.Bin, c.args("attach", "-t", SessionName(slug))...)
}

// DetachSession detaches any client attached to the session, leaving it running.
func (c *Client) DetachSession(slug string) error {
	_, err := c.run("detach-client", "-s", SessionName(slug))
	return err
}

func (c *Client) Kill(slug string) error {
	_, err := c.run("kill-session", "-t", SessionName(slug))
	return err
}

func startDir() string {
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return ""
}

// InstallHint names the platform's tmux install command.
func InstallHint() string {
	switch runtime.GOOS {
	case "darwin":
		return "brew install tmux"
	case "linux":
		if _, err := os.Stat("/etc/arch-release"); err == nil {
			return "sudo pacman -S tmux"
		}
		if _, err := os.Stat("/etc/alpine-release"); err == nil {
			return "sudo apk add tmux"
		}
		return "sudo apt install tmux"
	default:
		return "install tmux for your platform"
	}
}
