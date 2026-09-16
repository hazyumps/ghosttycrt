package tmux

import "testing"

// A live pane has an empty pane_dead_status, so its line ends in a tab. The
// parser must keep that pane; trimming the blob used to drop the last entry.
func TestParsePanesKeepsPanesWithEmptyTrailingFields(t *testing.T) {
	out := "%0\t\ttree\t0\t\n" +
		"%1\tcore-sw-01\ttree\t0\t\n" +
		"%2\tk3s-01\tk3s-01\t0\t\n"

	panes := parsePanes(out)
	if len(panes) != 3 {
		t.Fatalf("parsed %d panes, want 3: %+v", len(panes), panes)
	}
	if panes[2].Slug != "k3s-01" || panes[2].Window != "k3s-01" {
		t.Fatalf("last pane = %+v, want the k3s-01 one", panes[2])
	}
	if panes[0].ID != "%0" || panes[0].Slug != "" {
		t.Fatalf("tree pane = %+v", panes[0])
	}
	if panes[1].Visible() != true {
		t.Fatalf("core-sw-01 should be visible: %+v", panes[1])
	}
	if panes[2].Visible() != false {
		t.Fatalf("k3s-01 should be hidden: %+v", panes[2])
	}
}

func TestParsePanesReadsDeadStatus(t *testing.T) {
	panes := parsePanes("%7\tdies\ttree\t1\t3\n")
	if len(panes) != 1 {
		t.Fatalf("parsed %d panes, want 1", len(panes))
	}
	if !panes[0].Dead || panes[0].Exit != 3 {
		t.Fatalf("pane = %+v, want dead with status 3", panes[0])
	}
}

func TestParsePanesIgnoresShortLines(t *testing.T) {
	if got := parsePanes("garbage\n\n%1\ta\ttree\t0\t\n"); len(got) != 1 {
		t.Fatalf("parsed %+v, want only the well-formed line", got)
	}
}
