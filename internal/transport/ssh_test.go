package transport_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/hazyumps/ghosttycrt/internal/config"
	"github.com/hazyumps/ghosttycrt/internal/session"
	"github.com/hazyumps/ghosttycrt/internal/transport"
)

// Ask the installed SSH client to resolve the options without connecting.
// This catches precedence bugs that a string-only argv assertion would miss.
func TestSSHAuthMethodOverridesGlobalArguments(t *testing.T) {
	ssh, err := exec.LookPath("ssh")
	if err != nil {
		t.Skip("ssh not installed")
	}
	for _, method := range []string{"password", "publickey"} {
		t.Run(method, func(t *testing.T) {
			cfg := config.Default()
			cfg.Transports.SSH.Binary = ssh
			cfg.Transports.SSH.ExtraArgs = []string{"-o", "PubkeyAuthentication=yes", "-o", "PreferredAuthentications=publickey", "-o", "PasswordAuthentication=no", "-o", "KbdInteractiveAuthentication=no"}
			s := &session.Session{Transport: session.TransportSSH, SSH: &session.SSHConfig{Host: "example.invalid", AuthMethod: method}}
			tr, _ := transport.For(s, cfg)
			argv, _, err := tr.Argv(s, "")
			if err != nil {
				t.Fatal(err)
			}
			args := append([]string{"-G", "-F", "/dev/null"}, argv[1:]...)
			out, err := exec.Command(ssh, args...).Output()
			if err != nil {
				t.Fatal(err)
			}
			values := map[string]string{}
			for _, line := range strings.Split(string(out), "\n") {
				key, value, _ := strings.Cut(line, " ")
				values[key] = value
			}
			if method == "password" {
				if got := values["pubkeyauthentication"]; got != "no" && got != "false" {
					t.Fatalf("password login still offers keys: %q", got)
				}
				if values["preferredauthentications"] != "password,keyboard-interactive" {
					t.Fatal("password must be tried first")
				}
				for _, key := range []string{"passwordauthentication", "kbdinteractiveauthentication"} {
					if got := values[key]; got != "yes" && got != "true" {
						t.Fatalf("%s = %q", key, got)
					}
				}
			} else if values["preferredauthentications"] != "publickey" {
				t.Fatal("publickey login must not fall back to password")
			}
		})
	}
}

func TestSSHAuthMethodDefaultAndValidation(t *testing.T) {
	s := &session.Session{Transport: session.TransportSSH, SSH: &session.SSHConfig{Host: "example.invalid"}}
	tr, _ := transport.For(s, config.Default())
	for _, method := range []string{"", "auto"} {
		s.SSH.AuthMethod = method
		argv, _, err := tr.Argv(s, "")
		if err != nil || strings.Join(argv, " ") != "ssh example.invalid" {
			t.Fatalf("auto changed normal SSH: %v %v", argv, err)
		}
	}
	s.SSH.AuthMethod = "password; command"
	if _, _, err := tr.Argv(s, ""); err == nil {
		t.Fatal("accepted invalid authentication method")
	}
}

func sshSession() *session.Session {
	return &session.Session{
		ID: "1", Name: "core-sw-01", Slug: "core-sw-01", Transport: session.TransportSSH,
		SSH: &session.SSHConfig{
			Host:     "10.1.3.11",
			Port:     2222,
			User:     "admin",
			Jump:     "bastion",
			Identity: "~/.ssh/id_ed25519",
		},
	}
}

func TestSSHArgvBuildsEveryOption(t *testing.T) {
	tr, err := transport.For(sshSession(), config.Default())
	if err != nil {
		t.Fatal(err)
	}
	argv, _, err := tr.Argv(sshSession(), "")
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		"ssh",
		"-p", "2222",
		"-i", config.ExpandPath("~/.ssh/id_ed25519"),
		"-J", "bastion",
		"admin@10.1.3.11",
	}
	if strings.Join(argv, " ") != strings.Join(want, " ") {
		t.Fatalf("argv = %v\nwant    %v", argv, want)
	}
}

func TestSSHArgvOmitsDefaults(t *testing.T) {
	s := sshSession()
	s.SSH = &session.SSHConfig{Host: "esxi-02", Port: 22, User: "root"}

	tr, _ := transport.For(s, config.Default())
	argv, env, err := tr.Argv(s, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(argv, " ") != "ssh root@esxi-02" {
		t.Fatalf("argv = %v, want [ssh root@esxi-02]", argv)
	}
	if len(env) != 0 {
		t.Fatalf("env = %v, want empty", env)
	}
}

// AC-16: the secret must never be an argv element, because argv is
// world-readable in ps for the life of the process.
func TestSSHArgvNeverCarriesASecret(t *testing.T) {
	tr, _ := transport.For(sshSession(), config.Default())
	argv, _, err := tr.Argv(sshSession(), "correct-horse-battery-staple")
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range argv {
		if strings.Contains(a, "correct-horse-battery-staple") {
			t.Fatalf("secret leaked into argv: %v", argv)
		}
	}
}

func TestSSHArgvWithoutHostFails(t *testing.T) {
	s := sshSession()
	s.SSH = &session.SSHConfig{User: "admin"}

	tr, _ := transport.For(s, config.Default())
	if _, _, err := tr.Argv(s, ""); err == nil {
		t.Fatal("expected an error when ssh.host is empty")
	}
}

func TestSSHTerminalOverridesOnlyTheConnectionEnvironment(t *testing.T) {
	s := sshSession()
	s.SSH.Terminal = "xterm"
	tr, _ := transport.For(s, config.Default())
	argv, env, err := tr.Argv(s, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(env) != 0 || len(argv) < 3 || argv[0] != "env" || argv[1] != "TERM=xterm" || argv[2] != "ssh" {
		t.Fatalf("TERM must be applied after tmux starts the process: argv=%v env=%v", argv, env)
	}
	if strings.Contains(strings.Join(argv, " "), "SetEnv") {
		t.Fatal("TERM must be set locally before SSH requests its PTY")
	}
	for _, invalid := range []string{"xterm\n", "xterm;echo bad", "TERM=xterm", "-xterm", "xterm\x00"} {
		s.SSH.Terminal = invalid
		if _, _, err := tr.Argv(s, ""); err == nil {
			t.Errorf("accepted invalid terminal %q", invalid)
		}
	}
}

func TestExtraArgsAreAppended(t *testing.T) {
	cfg := config.Default()
	cfg.Transports.SSH.ExtraArgs = []string{"-o", "ServerAliveInterval=30"}

	s := sshSession()
	s.SSH = &session.SSHConfig{Host: "h", User: "u"}

	tr, _ := transport.For(s, cfg)
	argv, _, err := tr.Argv(s, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(argv, " "), "-o ServerAliveInterval=30") {
		t.Fatalf("extra_args missing from %v", argv)
	}
}

func TestUnimplementedTransportsSayWhichMilestone(t *testing.T) {
	for _, tr := range []session.Transport{session.TransportSerial, session.TransportTelnet, session.TransportLocal} {
		s := &session.Session{Transport: tr}
		_, err := transport.For(s, config.Default())
		if err == nil {
			t.Fatalf("transport %s should not be implemented yet", tr)
		}
		if !strings.Contains(err.Error(), "M2") {
			t.Errorf("transport %s: error %q should name the milestone", tr, err)
		}
	}
}
