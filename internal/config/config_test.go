package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hazyumps/ghosttycrt/internal/config"
)

func TestLoadMissingFileIsDefaults(t *testing.T) {
	cfg, err := config.Load(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatalf("a missing config should be defaults, not an error: %v", err)
	}
	if cfg.General.DisplayMode != config.DisplayInline {
		t.Fatalf("display_mode = %q, want %q", cfg.General.DisplayMode, config.DisplayInline)
	}
	if cfg.General.WorkspaceTreeWidth != 34 {
		t.Fatalf("workspace_tree_width = %d, want 34", cfg.General.WorkspaceTreeWidth)
	}
	if errs := cfg.Validate(); len(errs) != 0 {
		t.Fatalf("defaults should validate: %v", errs)
	}
}

func TestLoadMergesOverDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	body := `
[general]
display_mode = "workspace"
workspace_tree_width = 40

[transports.ssh]
extra_args = ["-o", "ServerAliveInterval=30"]
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.General.DisplayMode != config.DisplayWorkspace {
		t.Fatalf("display_mode = %q", cfg.General.DisplayMode)
	}
	if cfg.General.WorkspaceTreeWidth != 40 {
		t.Fatalf("workspace_tree_width = %d, want 40", cfg.General.WorkspaceTreeWidth)
	}
	// Unset keys keep their built-in values.
	if cfg.Transports.Serial.Backend != "picocom" {
		t.Fatalf("serial backend = %q, want picocom", cfg.Transports.Serial.Backend)
	}
	if cfg.Logging.RotateMB != 50 {
		t.Fatalf("rotate_mb = %d, want 50", cfg.Logging.RotateMB)
	}
}

func TestValidateRejects(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*config.Config)
	}{
		{"unknown display mode", func(c *config.Config) { c.General.DisplayMode = "teleport" }},
		{"tree width too small", func(c *config.Config) { c.General.WorkspaceTreeWidth = 4 }},
		{"tree width too large", func(c *config.Config) { c.General.WorkspaceTreeWidth = 500 }},
		{"unconfigured default provider", func(c *config.Config) { c.Credentials.DefaultProvider = "onepassword" }},
		{"provider without a command", func(c *config.Config) {
			c.Credentials.Providers["broken"] = config.Provider{}
			c.Credentials.DefaultProvider = "broken"
		}},
		{"bad serial backend", func(c *config.Config) { c.Transports.Serial.Backend = "minicom" }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Default()
			tc.mutate(cfg)
			if errs := cfg.Validate(); len(errs) == 0 {
				t.Fatalf("expected %s to be rejected", tc.name)
			}
		})
	}
}

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	if got := config.ExpandPath("~/.ssh/id_ed25519"); got != filepath.Join(home, ".ssh/id_ed25519") {
		t.Fatalf("ExpandPath = %q", got)
	}
	if got := config.ExpandPath("/absolute/path"); got != "/absolute/path" {
		t.Fatalf("ExpandPath changed an absolute path: %q", got)
	}
}
