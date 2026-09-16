package session

import (
	"testing"
)

func mk(name, group string, pinned bool) Session {
	return Session{
		ID:        name,
		Name:      name,
		Slug:      name,
		Transport: TransportSSH,
		Group:     group,
		Pinned:    pinned,
		SSH:       &SSHConfig{Host: "10.0.0.1"},
	}
}

func labels(rows []Row) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Node.Label)
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestBuildTreeNestsGroups(t *testing.T) {
	tree := BuildTree([]Session{
		mk("a", "network/switches", false),
		mk("b", "network/console", false),
		mk("c", "", false),
	})

	got := labels(tree.Flatten(nil, ""))
	want := []string{"network", "console", "b", "switches", "a", "c"}
	if !equal(got, want) {
		t.Fatalf("flatten = %v, want %v", got, want)
	}
}

func TestBuildTreeFloatsPinnedToTopOfGroup(t *testing.T) {
	tree := BuildTree([]Session{
		mk("alpha", "servers", false),
		mk("zeta", "servers", true),
	})

	got := labels(tree.Flatten(nil, ""))
	want := []string{"servers", "zeta", "alpha"}
	if !equal(got, want) {
		t.Fatalf("flatten = %v, want %v", got, want)
	}
}

func TestFlattenHonoursCollapse(t *testing.T) {
	tree := BuildTree([]Session{
		mk("a", "network/switches", false),
		mk("b", "servers", false),
	})

	collapsed := map[string]bool{"network": true}
	got := labels(tree.Flatten(collapsed, ""))
	want := []string{"network", "servers", "b"}
	if !equal(got, want) {
		t.Fatalf("flatten = %v, want %v", got, want)
	}
}

func TestFlattenFilterKeepsAncestorsAndForcesOpen(t *testing.T) {
	tree := BuildTree([]Session{
		mk("core-sw-01", "network/switches", false),
		mk("esxi-02", "servers", false),
	})

	collapsed := map[string]bool{"network": true}
	got := labels(tree.Flatten(collapsed, "coresw"))
	want := []string{"network", "switches", "core-sw-01"}
	if !equal(got, want) {
		t.Fatalf("flatten = %v, want %v", got, want)
	}
}

func TestSessionMatchesTagsAndGroup(t *testing.T) {
	s := mk("core-sw-01", "network/switches", false)
	s.Tags = []string{"cisco", "prod"}

	for _, q := range []string{"cisco", "network", "switches", "core", "1"} {
		if !s.Matches(q) {
			t.Errorf("Matches(%q) = false, want true", q)
		}
	}
	if s.Matches("zzz") {
		t.Error(`Matches("zzz") = true, want false`)
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Core SW 01":     "core-sw-01",
		"esxi-02":        "esxi-02",
		"  weird//name ": "weird-name",
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}
