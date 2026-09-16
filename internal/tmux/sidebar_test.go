package tmux

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestContentScriptIsValidShell(t *testing.T) {
	c := New("gcrt-test")
	script := c.contentScript()

	path := filepath.Join(t.TempDir(), "content.sh")
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("sh", "-n", path).CombinedOutput(); err != nil {
		t.Fatalf("the content pane's script does not parse: %v\n%s\n---\n%s", err, out, script)
	}
}

// The idle screen repeats on a timer so it settles at the right size, which is
// only safe because it clears first — the version that printed without clearing
// is what piled copies up the pane.
func TestContentScriptClearsBeforeDrawing(t *testing.T) {
	c := New("gcrt-test")
	script := c.contentScript()

	clear := strings.Index(script, `printf '\033[2J\033[H'`)
	if clear < 0 {
		t.Fatal("the empty state should clear the pane before drawing")
	}
	art := strings.Index(script, "GCRT_ART")
	if art < 0 || clear > art {
		t.Error("the clear must come before the wordmark is written")
	}
	if !strings.Contains(script, "sleep 0.5") {
		t.Error("the idle screen should pace its redraws")
	}
	for _, line := range idleArt {
		if !strings.Contains(script, strings.TrimSpace(line)) {
			t.Errorf("the wordmark is missing from the script: %q", line)
		}
	}
}

// The wordmark's glyphs carry their own left indent by design, so only the
// text lines are checked for centring.
func TestIdleBlockCentresItsText(t *testing.T) {
	block, width := idleBlock()

	for i := len(idleArt); i < len(block); i++ {
		line := block[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		want := (width - len([]rune(trimmed))) / 2
		got := len([]rune(line)) - len([]rune(strings.TrimLeft(line, " ")))
		if got != want {
			t.Errorf("line %d has %d leading spaces, want %d: %q", i, got, want, line)
		}
	}
	if !strings.Contains(strings.Join(block, "\n"), "no connections open yet") {
		t.Error("the empty state should say what it is")
	}
}

// The art lines are the widest thing in the block, so they set the width.
func TestIdleArtSetsTheBlockWidth(t *testing.T) {
	block, width := idleBlock()
	widest := 0
	for _, l := range idleArt {
		if n := len([]rune(l)); n > widest {
			widest = n
		}
	}
	if width < widest {
		t.Fatalf("block width = %d, narrower than the art's %d", width, widest)
	}
	if len(block) <= len(idleArt) {
		t.Fatal("the block should carry text under the wordmark")
	}
}

// The generated script is the only place % and stty quirks can hide.
func TestContentScriptHasNoEscapedPercent(t *testing.T) {
	c := New("gcrt-test")
	script := c.contentScript()
	// ${sz%% *} legitimately uses %%, but a printf format must not.
	if strings.Contains(script, "printf '%%") {
		t.Error("a printf format contains %%, which would be printed literally")
	}
	if !strings.Contains(script, "tr -d '\\r'") {
		t.Error("stty size output must have its carriage return stripped")
	}
}
