package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// box draws a lazydocker-style panel of outer size w x h with the title set
// into the top border. Lines are truncated or padded to fit.
func box(title string, lines []string, w, h int, focused bool) string {
	if w < 4 || h < 2 {
		return ""
	}
	border := lipgloss.NewStyle().Foreground(colorBorder)
	titleStyle := styleMuted
	if focused {
		border = lipgloss.NewStyle().Foreground(colorAccent)
		titleStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	}
	inner := w - 2

	t := ansi.Truncate(" "+title+" ", max(inner-1, 0), "…")
	fill := max(inner-1-ansi.StringWidth(t), 0)
	top := border.Render("┌─") + titleStyle.Render(t) + border.Render(strings.Repeat("─", fill)+"┐")

	var b strings.Builder
	b.WriteString(top)
	for i := range h - 2 {
		line := ""
		if i < len(lines) {
			line = fitWidth(lines[i], inner)
		} else {
			line = strings.Repeat(" ", inner)
		}
		b.WriteString("\n" + border.Render("│") + line + border.Render("│"))
	}
	b.WriteString("\n" + border.Render("└"+strings.Repeat("─", inner)+"┘"))
	return b.String()
}

// fitWidth truncates or right-pads s (which may contain ANSI) to exactly w cells.
func fitWidth(s string, w int) string {
	s = ansi.Truncate(s, w, "…")
	if pad := w - ansi.StringWidth(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}

// scrollWindow returns the [start, end) slice bounds that keep cursor visible
// in a list of n rows shown height at a time.
func scrollWindow(cursor, n, height int) (int, int) {
	if height <= 0 || n == 0 {
		return 0, 0
	}
	start := 0
	if cursor >= height {
		start = cursor - height + 1
	}
	return start, min(start+height, n)
}
