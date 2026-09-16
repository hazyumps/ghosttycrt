package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	General     General     `toml:"general"`
	Transports  Transports  `toml:"transports"`
	Logging     Logging     `toml:"logging"`
	Credentials Credentials `toml:"credentials"`
	Import      Import      `toml:"import"`
	Display     Display     `toml:"display"`
}

type General struct {
	DefaultTransport   string `toml:"default_transport"`
	DisplayMode        string `toml:"display_mode"`
	ConfirmOnQuit      bool   `toml:"confirm_on_quit"`
	SecretScrubLogs    bool   `toml:"secret_scrub_logs"`
	WorkspaceTreeWidth int    `toml:"workspace_tree_width"`
	WorkspaceLayout    string `toml:"workspace_layout"`
	Theme              Theme  `toml:"theme"`
}

type Theme struct {
	Name string `toml:"name"`
}

type Transports struct {
	SSH    SSHTransport    `toml:"ssh"`
	Serial SerialTransport `toml:"serial"`
	Telnet TelnetTransport `toml:"telnet"`
}

type SSHTransport struct {
	Binary    string   `toml:"binary"`
	ExtraArgs []string `toml:"extra_args"`
}

type SerialTransport struct {
	Backend     string `toml:"backend"`
	DefaultBaud int    `toml:"default_baud"`
}

type TelnetTransport struct {
	Binary string `toml:"binary"`
}

type Logging struct {
	EnabledByDefault bool   `toml:"enabled_by_default"`
	Directory        string `toml:"directory"`
	Timestamp        bool   `toml:"timestamp"`
	RotateMB         int    `toml:"rotate_mb"`
}

type Credentials struct {
	DefaultProvider string              `toml:"default_provider"`
	Providers       map[string]Provider `toml:"providers"`
}

// Provider is an argv template. {ref} is the only substitution and no shell is
// involved, so there is no word-splitting or quoting hazard.
type Provider struct {
	Command       []string `toml:"command"`
	UnlockCommand []string `toml:"unlock_command"`
}

type Import struct {
	SSHConfig SSHConfigImport `toml:"ssh_config"`
}

type SSHConfigImport struct {
	Enabled bool   `toml:"enabled"`
	Path    string `toml:"path"`
}

type Display struct {
	Ghostty GhosttyDisplay `toml:"ghostty"`
}

type GhosttyDisplay struct {
	CLIPath string `toml:"cli_path"`
}

const (
	DisplayInline        = "inline"
	DisplayGhosttyTab    = "ghostty-tab"
	DisplayGhosttyWindow = "ghostty-window"

	// DisplayWorkspace hosts the tree and every connection as panes of one
	// tmux session, so several sessions are live on screen at once.
	DisplayWorkspace = "workspace"
)

var validDisplayModes = []string{DisplayInline, DisplayGhosttyTab, DisplayGhosttyWindow, DisplayWorkspace}

// Workspace layouts: how a connection is shown beside the tree.
const (
	// WorkspaceTabs gives every connection its own tmux window — a tab, with
	// the tree in a tab of its own.
	WorkspaceTabs = "tabs"
	// WorkspaceSplit tiles connections as panes beside the tree, all visible
	// at once.
	WorkspaceSplit = "split"
)

var validWorkspaceLayouts = []string{WorkspaceTabs, WorkspaceSplit}

func Default() *Config {
	return &Config{
		General: General{
			DefaultTransport:   "ssh",
			DisplayMode:        DisplayInline,
			ConfirmOnQuit:      true,
			SecretScrubLogs:    true,
			WorkspaceTreeWidth: 34,
			WorkspaceLayout:    WorkspaceTabs,
			Theme:              Theme{Name: "auto"},
		},
		Transports: Transports{
			SSH:    SSHTransport{Binary: "ssh"},
			Serial: SerialTransport{Backend: "picocom", DefaultBaud: 9600},
			Telnet: TelnetTransport{Binary: "telnet"},
		},
		Logging: Logging{
			EnabledByDefault: false,
			Timestamp:        true,
			RotateMB:         50,
		},
		Credentials: Credentials{
			DefaultProvider: "infisical",
			Providers: map[string]Provider{
				"infisical": {
					Command: []string{"infisical", "secrets", "get", "{ref}", "--plain", "--env=prod"},
				},
				"vaultwarden": {
					Command:       []string{"rbw", "get", "{ref}"},
					UnlockCommand: []string{"rbw", "unlock"},
				},
			},
		},
		Import: Import{
			SSHConfig: SSHConfigImport{Enabled: true, Path: "~/.ssh/config"},
		},
		Display: Display{
			Ghostty: GhosttyDisplay{CLIPath: "/Applications/Ghostty.app/Contents/MacOS/ghostty"},
		},
	}
}

// Load decodes path over the built-in defaults, so a partial or absent file is
// a valid config rather than an error.
func Load(path string) (*Config, error) {
	cfg := Default()

	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}

	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

func (c *Config) Validate() []error {
	var errs []error

	if !contains(validDisplayModes, c.General.DisplayMode) {
		errs = append(errs, fmt.Errorf("general.display_mode %q is not one of %s",
			c.General.DisplayMode, strings.Join(validDisplayModes, ", ")))
	}
	if _, ok := c.Credentials.Providers[c.Credentials.DefaultProvider]; !ok {
		errs = append(errs, fmt.Errorf("credentials.default_provider %q is not a configured provider",
			c.Credentials.DefaultProvider))
	}
	for name, p := range c.Credentials.Providers {
		if len(p.Command) == 0 {
			errs = append(errs, fmt.Errorf("credentials.providers.%s.command is empty", name))
		}
	}
	if c.Transports.Serial.Backend != "picocom" && c.Transports.Serial.Backend != "screen" {
		errs = append(errs, fmt.Errorf("transports.serial.backend %q is not picocom or screen",
			c.Transports.Serial.Backend))
	}
	if c.Logging.RotateMB < 0 {
		errs = append(errs, fmt.Errorf("logging.rotate_mb must not be negative"))
	}
	if c.General.WorkspaceTreeWidth < 20 || c.General.WorkspaceTreeWidth > 200 {
		errs = append(errs, fmt.Errorf("general.workspace_tree_width %d must be between 20 and 200",
			c.General.WorkspaceTreeWidth))
	}
	if !contains(validWorkspaceLayouts, c.General.WorkspaceLayout) {
		errs = append(errs, fmt.Errorf("general.workspace_layout %q is not one of %s",
			c.General.WorkspaceLayout, strings.Join(validWorkspaceLayouts, ", ")))
	}
	return errs
}

func (c *Config) Provider(name string) (Provider, bool) {
	p, ok := c.Credentials.Providers[name]
	return p, ok
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// ExpandPath resolves a leading ~ and returns the path unchanged otherwise.
func ExpandPath(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(path, "~"), "/"))
		}
	}
	return path
}
