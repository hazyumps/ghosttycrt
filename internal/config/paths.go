package config

import (
	"os"
	"path/filepath"
)

// AppName is the directory name used under each XDG root.
const AppName = "ghosttycrt"

// Paths holds the XDG-resolved locations ghosttycrt reads and writes.
//
// XDG paths are used on macOS as well as Linux deliberately: they are portable
// and diffable, and ~/.config/ghosttycrt is the entire migration to another
// machine. See spec/02-architecture.md.
type Paths struct {
	Config string
	State  string
	Data   string
	Cache  string
}

func xdgRoot(env, fallback string) string {
	if v := os.Getenv(env); v != "" && filepath.IsAbs(v) {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fallback
	}
	return filepath.Join(home, fallback)
}

func DefaultPaths() Paths {
	return Paths{
		Config: filepath.Join(xdgRoot("XDG_CONFIG_HOME", ".config"), AppName),
		State:  filepath.Join(xdgRoot("XDG_STATE_HOME", filepath.Join(".local", "state")), AppName),
		Data:   filepath.Join(xdgRoot("XDG_DATA_HOME", filepath.Join(".local", "share")), AppName),
		Cache:  filepath.Join(xdgRoot("XDG_CACHE_HOME", ".cache"), AppName),
	}
}

func (p Paths) ConfigFile() string   { return filepath.Join(p.Config, "config.toml") }
func (p Paths) SessionsFile() string { return filepath.Join(p.Config, "sessions.toml") }
func (p Paths) StateFile() string    { return filepath.Join(p.State, "state.toml") }
func (p Paths) LogsDir() string      { return filepath.Join(p.Data, "logs") }
