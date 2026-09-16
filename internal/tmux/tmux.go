package tmux

import (
	"os/exec"
	"strconv"
	"strings"
)

// Socket is the private tmux socket used by gcrt. Keeping sessions off the
// default socket means gcrt never collides with a user's own tmux server, and
// tests can use their own with -L.
const Socket = "gcrt"

// Prefix namespaces gcrt sessions inside a socket.
const Prefix = "gscrt/"

func SessionName(slug string) string { return Prefix + slug }

func Available() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

type SessionState struct {
	Name     string
	Attached int
	Windows  int
	Created  int64
	Activity int64
}

func List(socket string) ([]SessionState, error) {
	out, err := exec.Command("tmux", "-L", socket, "list-sessions",
		"-F", "#{session_name}\t#{session_attached}\t#{session_windows}\t#{session_created}\t#{session_activity}").Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && strings.Contains(string(ee.Stderr), "no server running") {
			return nil, nil
		}
		return nil, err
	}

	var states []SessionState
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
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

// BySlug indexes live tmux sessions by gcrt slug.
func BySlug(socket string) (map[string]SessionState, error) {
	states, err := List(socket)
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
