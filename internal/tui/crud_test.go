package tui_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hazyumps/ghosttycrt/internal/config"
	"github.com/hazyumps/ghosttycrt/internal/session"
	"github.com/hazyumps/ghosttycrt/internal/tmux"
	"github.com/hazyumps/ghosttycrt/internal/tui"
)

// crudModel points the model at a scratch config directory, so a save writes a
// throwaway file and never the real session tree.
func crudModel(t *testing.T) (*tui.Model, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.toml")

	file := sample()
	if err := session.SaveFile(path, file.Session); err != nil {
		t.Fatal(err)
	}

	paths := config.DefaultPaths()
	paths.Config = dir

	m := tui.New(config.Default(), file, paths, session.Problems{}, tmux.New(unitSocket()))
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return updated.(*tui.Model), path
}

func reload(t *testing.T, path string) *session.File {
	t.Helper()
	file, problems, err := session.Load(path, []string{"infisical", "vaultwarden"})
	if err != nil {
		t.Fatalf("reloading %s: %v", path, err)
	}
	if !problems.OK() {
		t.Fatalf("the file we wrote does not validate: %v", problems.Strings())
	}
	return file
}

func ctrlS(t *testing.T, m *tui.Model) *tui.Model {
	t.Helper()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	return updated.(*tui.Model)
}

func byName(file *session.File, name string) (session.Session, bool) {
	for _, s := range file.Session {
		if s.Name == name {
			return s, true
		}
	}
	return session.Session{}, false
}

func TestNewSessionIsSavedAndAppearsInTheTree(t *testing.T) {
	m, path := crudModel(t)

	m, _ = press(t, m, "n")
	if !strings.Contains(m.View(), "new session") {
		t.Fatalf("n should open the form:\n%s", m.View())
	}

	m, _ = press(t, m, "lab-sw-07")
	m = pressKey(t, m, tea.KeyEnter) // accept the name field
	for i := 0; i < 6; i++ {         // name, group, tags, description, pinned, transport -> host
		m = pressKey(t, m, tea.KeyDown)
	}
	m, _ = press(t, m, "192.0.2.9")
	m = pressKey(t, m, tea.KeyEnter)
	m = ctrlS(t, m)

	if strings.Contains(m.View(), "new session") {
		t.Fatalf("a saved form should close:\n%s", m.View())
	}

	file := reload(t, path)
	got, ok := byName(file, "lab-sw-07")
	if !ok {
		t.Fatalf("lab-sw-07 is not in the file: %+v", file.Session)
	}
	if got.ID == "" || got.Slug != "lab-sw-07" {
		t.Fatalf("saved session has id %q and slug %q", got.ID, got.Slug)
	}
	if got.Transport != session.TransportSSH {
		t.Fatalf("transport = %q, want the configured default", got.Transport)
	}
	if out := m.View(); !strings.Contains(out, "lab-sw-07") {
		t.Errorf("the tree should show the new session:\n%s", out)
	}
}

// The file is the only copy of the session tree, so it is written 0600 by a
// temp-file-then-rename.
func TestSavedFileIsPrivateAndWellFormed(t *testing.T) {
	m, path := crudModel(t)
	m, _ = press(t, m, "n")
	m, _ = press(t, m, "priv-01")
	m = pressKey(t, m, tea.KeyEnter)
	m = ctrlS(t, m)

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("sessions.toml is %o, want 600", perm)
	}
	if body, err := os.ReadFile(path); err != nil || !strings.Contains(string(body), "[[session]]") {
		t.Errorf("file does not look like sessions.toml: %v", err)
	}
}

func TestAFormWithNoNameIsRefused(t *testing.T) {
	m, path := crudModel(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	m, _ = press(t, m, "n")
	m = ctrlS(t, m) // nothing filled in

	if !strings.Contains(m.View(), "name is required") {
		t.Fatalf("the form should say what is wrong:\n%s", m.View())
	}
	if !strings.Contains(m.View(), "new session") {
		t.Error("a rejected save should leave the form open")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("a rejected save wrote to the file")
	}
}

func TestEscapeLeavesTheFileAlone(t *testing.T) {
	m, path := crudModel(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	m, _ = press(t, m, "n")
	m, _ = press(t, m, "discard-me")
	m = pressKey(t, m, tea.KeyEscape)

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("cancelling the form wrote to the file")
	}
	if strings.Contains(m.View(), "discard-me") {
		t.Errorf("the cancelled session is still on screen:\n%s", m.View())
	}
}

func TestPinTogglesAndPersists(t *testing.T) {
	m, path := crudModel(t)

	before, _ := byName(reload(t, path), "core-sw-01")

	m = selectSession(t, m, "core-sw-01")
	m, _ = press(t, m, " ")
	after, ok := byName(reload(t, path), "core-sw-01")
	if !ok {
		t.Fatal("the session vanished")
	}
	if after.Pinned == before.Pinned {
		t.Fatalf("space did not flip pinned (still %v)", after.Pinned)
	}

	m = selectSession(t, m, "core-sw-01")
	m, _ = press(t, m, " ")
	back, _ := byName(reload(t, path), "core-sw-01")
	if back.Pinned != before.Pinned {
		t.Fatalf("pinned = %v, want it back at %v", back.Pinned, before.Pinned)
	}
}

func TestEditKeepsTheIDAndChangesTheName(t *testing.T) {
	m, path := crudModel(t)
	original, _ := byName(reload(t, path), "core-sw-01")

	m = selectSession(t, m, "core-sw-01")
	m, _ = press(t, m, "e")
	if !strings.Contains(m.View(), "edit core-sw-01") {
		t.Fatalf("e should open the form for the selection:\n%s", m.View())
	}

	// Clear the name field and type a new one.
	m = pressKey(t, m, tea.KeyEnter)
	for range "core-sw-01" {
		m = pressKey(t, m, tea.KeyBackspace)
	}
	m, _ = press(t, m, "core-sw-renamed")
	m = pressKey(t, m, tea.KeyEnter)
	m = ctrlS(t, m)

	file := reload(t, path)
	got, ok := byName(file, "core-sw-renamed")
	if !ok {
		t.Fatalf("rename did not take: %+v", file.Session)
	}
	if got.ID != original.ID {
		t.Errorf("id changed on rename: %s then %s", original.ID, got.ID)
	}
	if len(file.Session) != 2 {
		t.Errorf("rename should not change the count: %d", len(file.Session))
	}
}

func TestDuplicateGetsItsOwnIdentity(t *testing.T) {
	m, path := crudModel(t)

	m = selectSession(t, m, "core-sw-01")
	m, _ = press(t, m, "c")
	if out := m.View(); !strings.Contains(out, "core-sw-01 copy") {
		t.Fatalf("c should prefill a copy:\n%s", out)
	}
	m = ctrlS(t, m)

	file := reload(t, path)
	if len(file.Session) != 3 {
		t.Fatalf("got %d sessions, want 3", len(file.Session))
	}
	copy, ok := byName(file, "core-sw-01 copy")
	if !ok {
		t.Fatalf("the copy is missing: %+v", file.Session)
	}
	original, _ := byName(file, "core-sw-01")
	if copy.ID == original.ID {
		t.Error("the copy reused the original's id")
	}
	if copy.Slug == original.Slug || copy.Slug != "core-sw-01-copy" {
		t.Errorf("copy slug = %q", copy.Slug)
	}
	if copy.SSH == nil || copy.SSH.Host != original.SSH.Host {
		t.Error("the copy did not carry the ssh details over")
	}
}

func TestForgetRemovesTheRecord(t *testing.T) {
	m, path := crudModel(t)

	m = selectSession(t, m, "k3s-01")
	m, _ = press(t, m, "d")
	if out := m.View(); !strings.Contains(out, "Forget") {
		t.Fatalf("the menu should offer Forget:\n%s", out)
	}
	m = pressKey(t, m, tea.KeyEnter) // Forget is the only item when not running
	if out := m.View(); !strings.Contains(out, "Remove k3s-01") {
		t.Fatalf("expected a confirmation:\n%s", out)
	}
	m, _ = press(t, m, "y")

	file := reload(t, path)
	if _, ok := byName(file, "k3s-01"); ok {
		t.Fatalf("k3s-01 survived Forget: %+v", file.Session)
	}
	if len(file.Session) != 1 {
		t.Fatalf("got %d sessions, want 1", len(file.Session))
	}
}

func TestChangingTransportRevealsItsFields(t *testing.T) {
	m, _ := crudModel(t)
	m, _ = press(t, m, "n")

	// The transport field sits after the five general ones.
	for i := 0; i < 5; i++ {
		m = pressKey(t, m, tea.KeyDown)
	}
	if out := m.View(); !strings.Contains(out, "jump") {
		t.Fatalf("ssh fields should be showing:\n%s", out)
	}

	m, _ = press(t, m, " ") // cycle ssh -> serial
	if out := m.View(); !strings.Contains(out, "device") {
		t.Fatalf("serial fields should appear:\n%s", out)
	}
	if out := m.View(); strings.Contains(out, "jump") {
		t.Errorf("ssh fields should be gone:\n%s", out)
	}
}

func TestSlugCollisionsAreAvoided(t *testing.T) {
	m, path := crudModel(t)

	// core-sw-01 already exists, so a second one must not take its slug.
	m, _ = press(t, m, "n")
	m, _ = press(t, m, "core-sw-01")
	m = pressKey(t, m, tea.KeyEnter)
	for i := 0; i < 6; i++ {
		m = pressKey(t, m, tea.KeyDown)
	}
	m, _ = press(t, m, "192.0.2.10")
	m = pressKey(t, m, tea.KeyEnter)
	m = ctrlS(t, m)

	file := reload(t, path)
	seen := map[string]int{}
	for _, s := range file.Session {
		seen[s.Slug]++
		if seen[s.Slug] > 1 {
			t.Fatalf("duplicate slug %q in %+v", s.Slug, file.Session)
		}
	}
}

// ------------------------------------------------------------- group picker

// fieldIndex is where a labelled field sits in the form.
func formCursorRow(t *testing.T, m *tui.Model) int {
	t.Helper()
	for i, line := range strings.Split(m.View(), "\n") {
		if strings.Contains(stripANSI(line), "▸") {
			return i
		}
	}
	t.Fatalf("no field is focused:\n%s", m.View())
	return -1
}

func TestGroupFieldOffersTheExistingGroups(t *testing.T) {
	m, _ := crudModel(t)
	m, _ = press(t, m, "n")

	m = pressKey(t, m, tea.KeyDown)  // onto group
	m = pressKey(t, m, tea.KeyEnter) // open the picker

	out := m.View()
	for _, want := range []string{"network/switches", "servers", "(root)", "new group…"} {
		if !strings.Contains(out, want) {
			t.Errorf("the picker should offer %q:\n%s", want, out)
		}
	}
}

func TestPickingAGroupAndSaving(t *testing.T) {
	m, path := crudModel(t)

	m, _ = press(t, m, "n")
	m, _ = press(t, m, "grp-01")
	m = pressKey(t, m, tea.KeyEnter)
	m = pressKey(t, m, tea.KeyDown)  // group
	m = pressKey(t, m, tea.KeyEnter) // open the picker

	m, _ = clickAt(t, m, 10, rowOf(t, m, "servers"))
	for i := 0; i < 5; i++ {
		m = pressKey(t, m, tea.KeyDown) // group -> host
	}
	m, _ = press(t, m, "192.0.2.77")
	m = pressKey(t, m, tea.KeyEnter)
	m = ctrlS(t, m)

	got, ok := byName(reload(t, path), "grp-01")
	if !ok {
		t.Fatal("grp-01 was not saved")
	}
	if got.Group != "servers" {
		t.Fatalf("group = %q, want servers", got.Group)
	}
}

func TestThePickerCanTypeANewGroup(t *testing.T) {
	m, path := crudModel(t)

	m, _ = press(t, m, "n")
	m, _ = press(t, m, "grp-02")
	m = pressKey(t, m, tea.KeyEnter)
	m = pressKey(t, m, tea.KeyDown)
	m = pressKey(t, m, tea.KeyEnter) // open the picker
	m, _ = press(t, m, "lab/new")    // typing starts a path that does not exist
	m = pressKey(t, m, tea.KeyEnter)

	for i := 0; i < 5; i++ {
		m = pressKey(t, m, tea.KeyDown)
	}
	m, _ = press(t, m, "192.0.2.78")
	m = pressKey(t, m, tea.KeyEnter)
	m = ctrlS(t, m)

	got, ok := byName(reload(t, path), "grp-02")
	if !ok {
		t.Fatal("grp-02 was not saved")
	}
	if got.Group != "lab/new" {
		t.Fatalf("group = %q, want lab/new", got.Group)
	}
}

// ---------------------------------------------------------------- form mouse

func TestMouseSelectsAndOpensAField(t *testing.T) {
	m, _ := crudModel(t)
	m, _ = press(t, m, "n")

	desc := rowOf(t, m, "description")
	m, _ = clickAt(t, m, 10, desc)
	if got := formCursorRow(t, m); got != desc {
		t.Fatalf("a click should select the field on that row (%d), cursor is on %d", desc, got)
	}

	// A second click opens it for typing.
	m, _ = clickAt(t, m, 10, desc)
	m, _ = press(t, m, "note-to-self")
	if out := m.View(); !strings.Contains(out, "note-to-self") {
		t.Fatalf("a second click should start editing:\n%s", out)
	}
}

func TestMousePicksADropdownEntry(t *testing.T) {
	m, _ := crudModel(t)
	m, _ = press(t, m, "n")

	m = pressKey(t, m, tea.KeyDown)
	m = pressKey(t, m, tea.KeyEnter) // open the picker
	m, _ = clickAt(t, m, 10, rowOf(t, m, "network/switches"))

	if out := m.View(); !strings.Contains(out, "network/switches") {
		t.Fatalf("the chosen group should be in the field:\n%s", out)
	}
	// The list closes once something is chosen.
	if out := m.View(); strings.Contains(out, "new group…") {
		t.Errorf("the picker should close after a choice:\n%s", out)
	}
}

func TestWheelMovesBetweenFields(t *testing.T) {
	m, _ := crudModel(t)
	m, _ = press(t, m, "n")

	before := formCursorRow(t, m)
	updated, _ := m.Update(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonWheelDown,
		X:      10,
		Y:      5,
	})
	m = updated.(*tui.Model)

	if after := formCursorRow(t, m); after == before {
		t.Fatalf("the wheel should move the cursor; still on row %d", after)
	}
}
