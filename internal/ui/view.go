package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/hsk-kr/swingdesk/internal/model"
)

const (
	minWidth       = 60
	minHeight      = 14
	leftPaneWidth  = 22
	inboxHeightPct = 50
	timeLayout     = "02 Jan 15:04"
)

// View implements tea.Model.
func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = "swingdesk"
	return v
}

func (m Model) render() string {
	if m.width == 0 || m.height == 0 {
		return "loading…"
	}
	if m.width < minWidth || m.height < minHeight {
		return fmt.Sprintf("terminal too small (%dx%d); need at least %dx%d", m.width, m.height, minWidth, minHeight)
	}
	l := m.layout()
	left := box("Watchlist", m.filterLines(l.leftW-2, l.bodyH-2), l.leftW, l.bodyH, m.focus == paneFilters)
	inbox := box(m.inboxTitle(), m.inboxLines(l.rightW-2, l.inboxH-2), l.rightW, l.inboxH, m.focus == paneInbox)
	detailBody := m.detailLines(l.rightW - 2)
	if m.showHelp {
		detailBody = m.helpLines()
	}
	detail := box(m.detailTitle(), scroll(detailBody, m.detailScroll, l.detailH-2), l.rightW, l.detailH, m.focus == paneDetail || m.showHelp)

	right := inbox + "\n" + detail
	body := joinColumns(left, right)
	return m.header() + "\n" + body + "\n" + m.footer()
}

type layout struct {
	bodyH, leftW, rightW, inboxH, detailH int
}

func (m Model) layout() layout {
	bodyH := m.height - 2
	inboxH := bodyH * inboxHeightPct / 100
	return layout{
		bodyH:   bodyH,
		leftW:   leftPaneWidth,
		rightW:  m.width - leftPaneWidth,
		inboxH:  inboxH,
		detailH: bodyH - inboxH,
	}
}

// maxDetailScroll is the largest useful detail scroll offset.
func (m Model) maxDetailScroll() int {
	if m.width < minWidth || m.height < minHeight {
		return 0
	}
	l := m.layout()
	return max(len(m.detailLines(l.rightW-2))-(l.detailH-2), 0)
}

func (m Model) inboxTitle() string {
	return fmt.Sprintf("Inbox · %s · unread %d", m.filters[m.filterCursor].Label, len(m.visible))
}

func (m Model) detailTitle() string {
	if m.showHelp {
		return "Help"
	}
	return "Detail"
}

func (m Model) header() string {
	sep := styleMuted.Render(" │ ")
	parts := []string{
		styleBrand.Render("swingdesk"),
		styleHeader.Render(fmt.Sprintf("unread %d", len(m.items))),
		styleHeader.Render("last refresh " + m.fmtTimeOr(m.status.LastRefresh, "never")),
		styleHeader.Render("next refresh " + m.fmtTimeOr(m.status.NextRefresh, "—")),
		styleHeader.Render("agents " + m.status.Agents),
	}
	return fitWidth(" "+strings.Join(parts, sep), m.width)
}

func (m Model) footer() string {
	hints := []string{"j/k move", "h/l pane", "tab next", "enter open", "g/G top/bottom", "? help", "q quit"}
	return fitWidth(styleMuted.Render(" "+strings.Join(hints, " · ")), m.width)
}

func (m Model) fmtTimeOr(t time.Time, fallback string) string {
	if t.IsZero() {
		return fallback
	}
	return t.In(m.loc).Format("15:04")
}

func (m Model) filterLines(w, h int) []string {
	start, end := scrollWindow(m.filterCursor, len(m.filters), h)
	out := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		f := m.filters[i]
		n := countMatches(m.items, f)
		badge := ""
		if n > 0 {
			badge = fmt.Sprintf("%d", n)
		}
		label := fitWidth(" "+f.Label, w-len(badge)-1)
		line := label + badge + " "
		switch {
		case i == m.filterCursor && m.focus == paneFilters:
			line = styleSelected.Render(line)
		case i == m.filterCursor:
			line = styleSelDim.Render(line)
		default:
			line = styleText.Render(label) + styleBadge.Render(badge) + " "
		}
		out = append(out, line)
	}
	return out
}

func (m Model) inboxLines(w, h int) []string {
	if len(m.visible) == 0 {
		return []string{styleMuted.Render(" no unread items")}
	}
	start, end := scrollWindow(m.itemCursor, len(m.visible), h)
	out := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		it := m.visible[i]
		sym := it.Symbol
		if sym == "" {
			sym = "—"
		}
		symCol := fmt.Sprintf(" %-8s", ansi.Truncate(sym, 8, ""))
		catCol := fmt.Sprintf("%-10s", it.Category)
		titleW := max(w-ansi.StringWidth(symCol)-ansi.StringWidth(catCol)-1, 1)
		title := fitWidth(it.Title, titleW) + " "
		if i == m.itemCursor {
			style := styleSelDim
			if m.focus == paneInbox {
				style = styleSelected
			}
			out = append(out, style.Render(symCol+catCol+title))
			continue
		}
		out = append(out, styleBold.Render(symCol)+categoryStyle(it.Category).Render(catCol)+styleText.Render(title))
	}
	return out
}

func (m Model) detailLines(w int) []string {
	it, ok := m.Selected()
	if !ok {
		return []string{styleMuted.Render(" nothing selected")}
	}
	var lines []string
	add := func(s string) { lines = append(lines, s) }
	addWrapped := func(s string, style func(...string) string) {
		for _, l := range strings.Split(ansi.Wordwrap(s, max(w-2, 10), " -"), "\n") {
			add(" " + style(l))
		}
	}

	addWrapped(it.Title, styleHeading.Render)
	meta := []string{}
	if it.Symbol != "" {
		meta = append(meta, it.Symbol)
	}
	meta = append(meta, string(it.Category))
	if it.Source != "" {
		meta = append(meta, it.Source)
	}
	if !it.PublishedAt.IsZero() {
		meta = append(meta, "published "+it.PublishedAt.In(m.loc).Format(timeLayout))
	}
	add(" " + styleMuted.Render(strings.Join(meta, " · ")))
	if it.URL != "" {
		add(" " + styleMuted.Render(it.URL))
	}
	add("")
	if it.Summary != "" {
		addWrapped(it.Summary, styleText.Render)
	}
	if it.Body != "" {
		add("")
		addWrapped(it.Body, styleText.Render)
	}
	return append(lines, m.biasLines(it, w)...)
}

func (m Model) biasLines(it model.Item, w int) []string {
	b, ok := m.biases[it.InstrumentID]
	if it.InstrumentID == 0 || !ok {
		return nil
	}
	sym := m.instruments[it.InstrumentID].Symbol
	out := []string{
		"",
		fmt.Sprintf(" %s %s  %s",
			styleMuted.Render(sym+" stance:"),
			stanceStyle(b.Stance).Render(string(b.Stance)),
			styleMuted.Render(fmt.Sprintf("conf %.2f", b.Confidence))),
	}
	for _, l := range strings.Split(ansi.Wordwrap(b.Rationale, max(w-2, 10), " -"), "\n") {
		if l != "" {
			out = append(out, " "+styleText.Render(l))
		}
	}
	return out
}

func (m Model) helpLines() []string {
	return []string{
		" " + styleHeading.Render("Keys"),
		" j/k ↑/↓    move / scroll",
		" h/l ←/→    change pane",
		" tab        next pane",
		" enter      open detail",
		" esc        back",
		" g / G      top / bottom",
		" ? / esc    close help",
		" q          quit",
		"",
		" " + styleMuted.Render("Full keybinding list lands with the polish pass."),
	}
}

// scroll returns up to h lines starting at offset (clamped so the last page
// stays full).
func scroll(lines []string, offset, h int) []string {
	if h <= 0 {
		return nil
	}
	offset = clamp(offset, 0, max(len(lines)-h, 0))
	return lines[offset:min(offset+h, len(lines))]
}

// joinColumns places two multi-line blocks side by side.
func joinColumns(left, right string) string {
	l := strings.Split(left, "\n")
	r := strings.Split(right, "\n")
	n := max(len(l), len(r))
	var b strings.Builder
	for i := range n {
		if i > 0 {
			b.WriteByte('\n')
		}
		if i < len(l) {
			b.WriteString(l[i])
		}
		if i < len(r) {
			b.WriteString(r[i])
		}
	}
	return b.String()
}
