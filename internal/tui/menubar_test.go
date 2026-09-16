package tui_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hazyumps/ghosttycrt/internal/tui"
)

func clickAt(t *testing.T, m *tui.Model, x, y int) (*tui.Model, tea.Cmd) {
	t.Helper()
	updated, cmd := m.Update(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      x,
		Y:      y,
	})
	return updated.(*tui.Model), cmd
}

// barCol finds a menu bar item's column by looking at what was actually drawn,
// so the test does not have to duplicate the layout maths. It searches only
// after the brand, or a single-letter label would match a letter in "gcrt".
func barCol(t *testing.T, m *tui.Model, wide, narrow string) int {
	t.Helper()
	header := stripANSI(strings.Split(m.View(), "\n")[0])

	brandEnd := 0
	for _, brand := range []string{" ghosttycrt ", " gcrt "} {
		if i := strings.Index(header, brand); i >= 0 {
			brandEnd = len([]rune(header[:i])) + len([]rune(brand))
			break
		}
	}
	rest := string([]rune(header)[brandEnd:])

	for _, name := range []string{wide, narrow} {
		if i := strings.Index(rest, name); i >= 0 {
			return brandEnd + len([]rune(rest[:i]))
		}
	}
	t.Fatalf("neither %q nor %q appears after the brand in %q", wide, narrow, header)
	return 0
}

func TestMenuBarIsRenderedWithItsItems(t *testing.T) {
	m := newModel(t, sample(), 34, 20)
	header := strings.Split(m.View(), "\n")[0]

	for _, want := range []string{"gcrt", "?", "/", "r", "q"} {
		if !strings.Contains(header, want) {
			t.Errorf("menu bar is missing %q:\n%s", want, header)
		}
	}
}

func TestWideMenuBarUsesFullLabels(t *testing.T) {
	m := newModel(t, sample(), 100, 30)
	header := strings.Split(m.View(), "\n")[0]

	for _, want := range []string{"ghosttycrt", "Help", "Filter", "Refresh", "Quit"} {
		if !strings.Contains(header, want) {
			t.Errorf("wide menu bar is missing %q:\n%s", want, header)
		}
	}
}

func TestClickingTheMenuBarOpensHelp(t *testing.T) {
	for _, width := range []int{34, 100} {
		m := newModel(t, sample(), width, 20)
		x := barCol(t, m, "Help", "?")

		m, cmd := clickAt(t, m, x, 0)
		if cmd != nil {
			t.Fatalf("width %d: opening help should not return a command", width)
		}
		if out := m.View(); !strings.Contains(out, "Moving") {
			t.Fatalf("width %d: clicking help did not open the help screen:\n%s", width, out)
		}
	}
}

func TestClickingTheMenuBarStartsFiltering(t *testing.T) {
	m := newModel(t, sample(), 34, 20)

	m, _ = clickAt(t, m, barCol(t, m, "Filter", "/"), 0)
	m, _ = press(t, m, "k3s")

	out := m.View()
	if !strings.Contains(out, "k3s-01") {
		t.Fatalf("filter did not engage:\n%s", out)
	}
	if strings.Contains(out, "core-sw-01") {
		t.Error("filter did not narrow the tree")
	}
}

// A click on the menu bar must not fall through and open whatever session sits
// under those columns in the tree.
func TestClickingTheMenuBarDoesNotOpenASession(t *testing.T) {
	// A fresh model per item: clicking Help opens a modal, which replaces the
	// header the next column lookup would search.
	for _, item := range [][2]string{{"Help", "?"}, {"Filter", "/"}, {"Refresh", "r"}} {
		m := newModel(t, sample(), 34, 20)
		x := barCol(t, m, item[0], item[1])
		if _, cmd := clickAt(t, m, x, 0); cmd != nil {
			t.Fatalf("clicking the %s item returned a command", item[0])
		}
	}
}

func TestHelpOpensFromTheQuestionKeyToo(t *testing.T) {
	m := newModel(t, sample(), 34, 20)
	m, _ = press(t, m, "?")
	if out := m.View(); !strings.Contains(out, "Moving") {
		t.Fatalf("? should still open help:\n%s", out)
	}
}

// Bubbletea hands over several keystrokes in one message when they arrive
// together, which is what a paste or fast typing looks like.
func TestBatchedKeystrokesAreNotDropped(t *testing.T) {
	m := newModel(t, sample(), 34, 20)

	// Rows: network, switches, core-sw-01, servers, k3s-01. Two downs from the
	// first session reach the last one.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("jj")})
	m = updated.(*tui.Model)
	if got := m.SelectedName(); got != "k3s-01" {
		t.Fatalf("batched 'jj' left the cursor on %q, want k3s-01", got)
	}

	// A paste into the filter must land whole.
	m = newModel(t, sample(), 34, 20)
	m, _ = press(t, m, "/")
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k3s")})
	m = updated.(*tui.Model)
	if out := m.View(); !strings.Contains(out, "k3s-01") || strings.Contains(out, "core-sw-01") {
		t.Fatalf("pasted filter text was dropped:\n%s", out)
	}
}

// A batched '?G' is exactly what broke this: '?' opens help, then 'G' jumps to
// the end of it.
func TestBatchedKeysOpenAndScrollHelp(t *testing.T) {
	m := newModel(t, sample(), 34, 20)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?G")})
	m = updated.(*tui.Model)

	if out := m.View(); !strings.Contains(out, "Commands") {
		t.Fatalf("batched '?G' should open help and jump to the end:\n%s", out)
	}
}

func TestHelpScrollsToTheLastSection(t *testing.T) {
	m := newModel(t, sample(), 34, 20)
	m, _ = press(t, m, "?")
	if !strings.Contains(m.View(), "Moving") {
		t.Fatalf("help should start at the top:\n%s", m.View())
	}

	m, _ = press(t, m, "G")
	if out := m.View(); !strings.Contains(out, "Commands") {
		t.Fatalf("G should jump to the end of the help:\n%s", out)
	}

	m, _ = press(t, m, "g")
	if out := m.View(); !strings.Contains(out, "Moving") {
		t.Fatalf("g should jump back to the top:\n%s", out)
	}

	m, _ = press(t, m, "kkkkk")
	if out := m.View(); !strings.Contains(out, "Moving") {
		t.Fatalf("scrolling up past the top should clamp rather than blank:\n%s", out)
	}
}

func TestWheelScrollsTheHelp(t *testing.T) {
	m := newModel(t, sample(), 34, 20)
	m, _ = press(t, m, "?")

	before := m.View()
	updated, _ := m.Update(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonWheelDown,
		X:      5,
		Y:      5,
	})
	if updated.(*tui.Model).View() == before {
		t.Error("the wheel should scroll an open help screen")
	}
}

// Every rendered line must fit the pane. The footer's key list alone is longer
// than 80 columns, and it used to be composed without clipping, so the whole
// frame wrapped.
func TestViewNeverExceedsThePaneWidth(t *testing.T) {
	for _, width := range []int{34, 50, 60, 80, 100, 120} {
		for _, state := range []string{"tree", "help"} {
			m := newModel(t, sample(), width, 24)
			if state == "help" {
				m, _ = press(t, m, "?")
			}

			view := m.View()
			for i, line := range strings.Split(view, "\n") {
				if w := len([]rune(stripANSI(line))); w > width {
					t.Errorf("%dx24 %s: line %d is %d columns wide:\n%q",
						width, state, i, w, stripANSI(line))
				}
			}
			if got := len(strings.Split(view, "\n")); got > 24 {
				t.Errorf("%dx24 %s: rendered %d lines", width, state, got)
			}
		}
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			i++ // step over the terminating 'm'
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
