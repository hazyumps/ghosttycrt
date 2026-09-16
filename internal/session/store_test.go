package session

import "testing"

func TestValidateAcceptsAWellFormedSession(t *testing.T) {
	f := &File{Session: []Session{
		{
			ID:        "id-1",
			Name:      "core-sw-01",
			Slug:      "core-sw-01",
			Transport: TransportSSH,
			SSH:       &SSHConfig{Host: "10.1.3.11", User: "admin"},
			Credentials: &CredentialsConfig{
				Provider: "infisical",
				Ref:      "CORE-SW-01-PASS",
			},
		},
	}}

	if p := f.Validate([]string{"infisical"}); !p.OK() {
		t.Fatalf("unexpected errors: %v", p.Strings())
	}
}

func TestValidateCatches(t *testing.T) {
	cases := []struct {
		name    string
		session Session
		provs   []string
	}{
		{
			name:    "missing id",
			session: Session{Name: "a", Slug: "a", Transport: TransportSSH, SSH: &SSHConfig{Host: "h"}},
		},
		{
			name:    "bad slug",
			session: Session{ID: "1", Name: "a", Slug: "Bad Slug", Transport: TransportSSH, SSH: &SSHConfig{Host: "h"}},
		},
		{
			name:    "duplicate slug",
			session: Session{ID: "2", Name: "b", Slug: "a", Transport: TransportSSH, SSH: &SSHConfig{Host: "h"}},
		},
		{
			name: "transport mismatch",
			session: Session{
				ID: "3", Name: "c", Slug: "c", Transport: TransportSSH,
				Serial: &SerialConfig{Device: "/dev/null", Baud: 9600},
			},
		},
		{
			name: "bad baud",
			session: Session{
				ID: "4", Name: "d", Slug: "d", Transport: TransportSerial,
				Serial: &SerialConfig{Device: "/dev/null", Baud: 12345},
			},
		},
		{
			name: "unconfigured provider",
			session: Session{
				ID: "5", Name: "e", Slug: "e", Transport: TransportSSH, SSH: &SSHConfig{Host: "h"},
				Credentials: &CredentialsConfig{Provider: "onepassword", Ref: "x"},
			},
		},
		{
			name: "literal secret as ref",
			session: Session{
				ID: "6", Name: "f", Slug: "f", Transport: TransportSSH, SSH: &SSHConfig{Host: "h"},
				Credentials: &CredentialsConfig{Provider: "infisical", Ref: "AbCd1234EfGh5678IjKl9012"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &File{Session: append([]Session{
				{ID: "dup", Name: "a", Slug: "a", Transport: TransportSSH, SSH: &SSHConfig{Host: "h"}},
			}, tc.session)}

			provs := tc.provs
			if provs == nil {
				provs = []string{"infisical", "vaultwarden"}
			}
			if p := f.Validate(provs); p.OK() {
				t.Fatalf("expected an error for %s", tc.name)
			}
		})
	}
}

func TestValidateWarnsOnMissingSerialDevice(t *testing.T) {
	f := &File{Session: []Session{{
		ID: "1", Name: "console", Slug: "console", Transport: TransportSerial,
		Serial: &SerialConfig{Device: "/dev/tty.does-not-exist", Baud: 9600},
	}}}

	p := f.Validate([]string{"infisical"})
	if !p.OK() {
		t.Fatalf("unexpected errors: %v", p.Strings())
	}
	if len(p.Warnings) != 1 {
		t.Fatalf("warnings = %v, want 1", p.Strings())
	}
}

func TestCredentialsMustBeNoneOrProvider(t *testing.T) {
	f := &File{Session: []Session{{
		ID: "1", Name: "a", Slug: "a", Transport: TransportSSH, SSH: &SSHConfig{Host: "h"},
		Credentials: &CredentialsConfig{Provider: "none", Ref: "SOMETHING"},
	}}}
	if p := f.Validate([]string{"infisical"}); p.OK() {
		t.Fatal("expected an error when provider is none but ref is set")
	}
}
