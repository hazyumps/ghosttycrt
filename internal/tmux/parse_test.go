package tmux

import "testing"

// A live pane has an empty pane_dead_status, so its line carries an empty field.
// The parser must keep that pane; trimming the blob used to drop the last entry.
func TestParsePanesKeepsPanesWithEmptyTrailingFields(t *testing.T) {
	out := "%0\t\ttree\t0\t\t1\n" +
		"%1\tcore-sw-01\ttree\t0\t\t0\n" +
		"%2\tk3s-01\tk3s-01\t0\t\t0\n"

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
	if !panes[0].WindowActive {
		t.Error("the tree window should read as active")
	}
	if panes[1].WindowActive {
		t.Error("a background tab should not read as active")
	}
}

func TestParsePanesReadsDeadStatus(t *testing.T) {
	panes := parsePanes("%7\tdies\tdies\t1\t3\t0\n")
	if len(panes) != 1 {
		t.Fatalf("parsed %d panes, want 1", len(panes))
	}
	if !panes[0].Dead || panes[0].Exit != 3 {
		t.Fatalf("pane = %+v, want dead with status 3", panes[0])
	}
}

func TestParsePanesIgnoresShortLines(t *testing.T) {
	if got := parsePanes("garbage\n\n%1\ta\ttree\t0\t\t0\n"); len(got) != 1 {
		t.Fatalf("parsed %+v, want only the well-formed line", got)
	}
}

// Split layout means "tiled beside the tree"; tabs means "its window is front".
func TestPaneOpenDependsOnLayout(t *testing.T) {
	beside := Pane{Window: TreeWindow, WindowActive: false}
	hidden := Pane{Window: "k3s-01", WindowActive: false}
	front := Pane{Window: "k3s-01", WindowActive: true}

	if !beside.Open(LayoutSplit) || hidden.Open(LayoutSplit) {
		t.Error("split layout: only a pane in the tree window is open")
	}
	if !front.Open(LayoutTabs) || hidden.Open(LayoutTabs) {
		t.Error("tabs layout: only the current window is open")
	}
	if !beside.Visible() {
		t.Error("Visible still means tiled beside the tree")
	}
}
