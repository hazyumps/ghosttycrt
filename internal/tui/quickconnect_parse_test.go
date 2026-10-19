package tui

import "testing"

func TestParseQuickAddress(t *testing.T) {
	for _, tc := range []struct {
		address, host, user string
		port                int
	}{
		{"edge", "edge", "", 22},
		{"admin@switch.example:2222", "switch.example", "admin", 2222},
		{" ssh://admin@192.0.2.4:2200 ", "192.0.2.4", "admin", 2200},
		{"admin@[2001:db8::4]:2222", "2001:db8::4", "admin", 2222},
		{"2001:db8::4", "2001:db8::4", "", 22},
		{"admin@2001:db8::4", "2001:db8::4", "admin", 22},
	} {
		t.Run(tc.address, func(t *testing.T) {
			got, err := parseQuickAddress(tc.address)
			if err != nil {
				t.Fatal(err)
			}
			if got.Host != tc.host || got.User != tc.user || got.Port != tc.port {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestQuickAddressRejectsPasswordsOptionsAndMalformedPorts(t *testing.T) {
	for _, address := range []string{"", "@host", "user@", "host:0", "host:65536", "host:", "host:abc", "https://host", "user:password@host", "host/path", "host?x=y", "host#fragment", "-oProxyCommand=bad", "-lroot@host", "host command", "host\ncommand", "user%0Acommand@host"} {
		t.Run(address, func(t *testing.T) {
			if got, err := parseQuickAddress(address); err == nil {
				t.Fatalf("accepted invalid address as %+v", got)
			}
		})
	}
}
