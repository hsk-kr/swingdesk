package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"github.com/charmbracelet/x/ansi"
)

// disclaimer is shown in the footer and the help modal.
const disclaimer = "research hint, not financial advice"

const helpMaxWidth = 64

// helpLines is the full keybinding reference, built from the key map so it
// cannot drift from the bindings.
func (m Model) helpLines() []string {
	k := m.keys
	sections := []struct {
		title    string
		bindings []key.Binding
	}{
		{"Navigate", []key.Binding{k.Down, k.Up, k.Left, k.Right, k.NextPane, k.PrevPane, k.Enter, k.Back, k.Top, k.Bottom}},
		{"Inbox", []key.Binding{k.MarkRead, k.MarkAll, k.Undo, k.ShowRead, k.Filter, k.Copy}},
		{"Refresh", []key.Binding{k.Refresh}},
		{"Other", []key.Binding{k.Help, k.Quit}},
	}
	var lines []string
	for _, s := range sections {
		lines = append(lines, " "+styleHeading.Render(s.title))
		for _, b := range s.bindings {
			h := b.Help()
			lines = append(lines, fmt.Sprintf("   %-10s %s", h.Key, h.Desc))
		}
		lines = append(lines, "")
	}
	return append(lines,
		" "+styleHeading.Render("Watch the agents"),
		"   tmux attach -t "+m.sessionName(),
		"   "+styleMuted.Render("one window per job: market, tech, names"),
		"",
		" "+styleDisclaimer.Render("Stances are a "+disclaimer+"."),
	)
}

func (m Model) sessionName() string {
	if m.session == "" {
		return "swingdesk"
	}
	return m.session
}

// helpSize is the modal's outer width and height.
func (m Model) helpSize() (int, int) {
	return min(helpMaxWidth, m.width-4), min(len(m.helpLines())+2, m.height-2)
}

// maxHelpScroll is the largest useful help scroll offset.
func (m Model) maxHelpScroll() int {
	if m.width < minWidth || m.height < minHeight {
		return 0
	}
	_, h := m.helpSize()
	return max(len(m.helpLines())-(h-2), 0)
}

func (m Model) helpModal() string {
	w, h := m.helpSize()
	return box("Help · j/k scroll · ? or esc close", scroll(m.helpLines(), m.helpScroll, h-2), w, h, true)
}

// overlay centres modal over base (both multi-line, ANSI-styled).
func overlay(base, modal string, width int) string {
	rows := strings.Split(base, "\n")
	lines := strings.Split(modal, "\n")
	mw := ansi.StringWidth(lines[0])
	x := max((width-mw)/2, 0)
	y := max((len(rows)-len(lines))/2, 0)
	out := make([]string, len(rows))
	copy(out, rows)
	for i, l := range lines {
		if y+i >= len(out) {
			break
		}
		out[y+i] = spliceRow(out[y+i], l, x, mw, width)
	}
	return strings.Join(out, "\n")
}

// spliceRow replaces cells [x, x+mw) of row with l, keeping the row exactly
// width cells even when a double-width character straddles either edge
// (the straddling character is replaced by spaces).
func spliceRow(row, l string, x, mw, width int) string {
	left := ansi.Truncate(row, x, "")
	left += strings.Repeat(" ", max(x-ansi.StringWidth(left), 0))
	want := max(width-x-mw, 0)
	right := ansi.TruncateLeft(row, x+mw, "")
	if ansi.StringWidth(right) > want {
		// A wide char straddles the cut; the next cell is a boundary.
		right = ansi.TruncateLeft(row, x+mw+1, "")
	}
	return left + l + strings.Repeat(" ", max(want-ansi.StringWidth(right), 0)) + right
}
