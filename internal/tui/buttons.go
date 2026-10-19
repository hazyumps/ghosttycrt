package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type button struct {
	label string
	run   func() tea.Cmd
}

type buttonHit struct {
	x, y, end int
	run       func() tea.Cmd
}

// Rendering owns the hit boxes, including both axes. Blank space beside a
// destructive action must never activate it.
func (m *Model) buttonLine(actions []button, x, y, width int) string {
	var out strings.Builder
	col := 0
	for _, action := range actions {
		label := "[ " + action.label + " ]"
		n := lipgloss.Width(label)
		if col+n > width {
			break
		}
		style := styleText.Bold(true).Background(colHover)
		if m.hovering(y, x+col, x+col+n) {
			style = styleBarHi
		}
		out.WriteString(style.Render(label))
		m.buttons = append(m.buttons, buttonHit{x: x + col, y: y, end: x + col + n, run: action.run})
		out.WriteString("  ")
		col += n + 2
	}
	return strings.TrimRight(out.String(), " ")
}
