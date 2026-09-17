package tui_test

import (
	"os"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Without a terminal lipgloss renders no colour at all, which would make every
// styling assertion in these tests vacuously pass. Force a profile so hover,
// the cursor and the menu bar are actually exercised.
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	os.Exit(m.Run())
}
