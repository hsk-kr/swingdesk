package ui

import (
	"fmt"
	"slices"
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
	detail := box("Detail", scroll(m.detailLines(l.rightW-2), m.detailScroll, l.detailH-2), l.rightW, l.detailH, m.focus == paneDetail)

	frame := m.header() + "\n" + joinColumns(left, inbox+"\n"+detail) + "\n" + m.footer()
	if m.showHelp {
		return overlay(frame, m.helpModal(), m.width)
	}
	return frame
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
	f := m.filters[m.filterCursor]
	title := fmt.Sprintf("Inbox · %s · unread %d", f.Label, m.counts.Count(f.Query))
	if m.showRead {
		title += " · incl. read"
	}
	if m.query != "" {
		title += fmt.Sprintf(" · /%s (%d)", m.query, len(m.visible))
	}
	return title
}

func (m Model) header() string {
	parts := []part{
		{styleBrand.Render("swingdesk"), 0},
		{styleHeader.Render(fmt.Sprintf("unread %d", m.counts.Total)), 1},
	}
	if m.flashN > 0 && m.clock.Before(m.flashUntil) {
		parts = append(parts, part{styleFlash.Render(fmt.Sprintf("+%d new", m.flashN)), 1})
	}
	parts = append(parts,
		part{styleHeader.Render("last refresh " + m.fmtTimeOr(m.status.LastRefresh, "never")), 4},
		part{styleHeader.Render("next " + m.nextRefreshText()), 3},
		part{m.agentsText(), 2},
	)
	return fitWidth(" "+joinFitting(parts, styleMuted.Render(" │ "), m.width-1), m.width)
}

// nextRefreshText is "12:11 (in 28:14)", or "—" before scheduling.
func (m Model) nextRefreshText() string {
	next := m.status.NextRefresh
	if next.IsZero() {
		return "—"
	}
	left := next.Sub(m.clock)
	if left < 0 || m.sched.Running() {
		return m.fmtTimeOr(next, "—")
	}
	return fmt.Sprintf("%s (in %s)", m.fmtTimeOr(next, "—"), fmtCountdown(left))
}

// fmtCountdown renders d as m:ss, or h:mm:ss past an hour.
func fmtCountdown(d time.Duration) string {
	sec := int(d.Round(time.Second).Seconds())
	h, mm, ss := sec/3600, sec%3600/60, sec%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, mm, ss)
	}
	return fmt.Sprintf("%d:%02d", mm, ss)
}

// maxAgentsWidth caps the header's agent text; full errors go to the footer.
const maxAgentsWidth = 40

func (m Model) agentsText() string {
	text := ansi.Truncate("agents "+m.status.Agents.String(), maxAgentsWidth, "…")
	switch m.status.Agents.State {
	case model.AgentRunning:
		return styleFlash.Render(text)
	case model.AgentError:
		return styleError.Render(text)
	default:
		return styleHeader.Render(text)
	}
}

func (m Model) footer() string {
	if m.typing {
		return fitWidth(styleFlash.Render(" /"+m.query+"█")+styleMuted.Render("  enter keep · esc clear"), m.width)
	}
	if m.notice != "" {
		style := styleNotice
		if m.noticeErr {
			style = styleError
		}
		return fitWidth(style.Render(" "+m.notice), m.width)
	}
	parts := []part{
		{"j/k move", 2}, {"r read", 1}, {"/ filter", 3}, {"c copy", 4}, {"R refresh", 3},
		{"? help", 0}, {"q quit", 0}, {styleDisclaimer.Render(disclaimer), 0},
	}
	return fitWidth(styleMuted.Render(" "+joinFitting(parts, " · ", m.width-1)), m.width)
}

// part is a header/footer segment; higher drop values are dropped first when
// the line does not fit.
type part struct {
	text string
	drop int
}

// joinFitting joins parts with sep, dropping the highest-drop parts (keeping
// order) until the result fits in width.
func joinFitting(parts []part, sep string, width int) string {
	keep := slices.Clone(parts)
	for {
		texts := make([]string, len(keep))
		for i, p := range keep {
			texts[i] = p.text
		}
		line := strings.Join(texts, sep)
		if ansi.StringWidth(line) <= width || len(keep) <= 1 {
			return line
		}
		worst := 0
		for i, p := range keep {
			if p.drop >= keep[worst].drop {
				worst = i
			}
		}
		keep = slices.Delete(keep, worst, worst+1)
	}
}

// fmtDateTimeOr includes the date so stale data never reads as today.
func (m Model) fmtDateTimeOr(t time.Time, fallback string) string {
	if t.IsZero() {
		return fallback
	}
	return t.In(m.loc).Format(timeLayout)
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
		n := m.counts.Count(f.Query)
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
		return m.emptyInbox()
	}
	start, end := scrollWindow(m.itemCursor, len(m.visible), h)
	out := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		it := m.visible[i]
		sym := it.Symbol
		if sym == "" {
			sym = "—"
		}
		mark := " "
		if !it.ReadAt.IsZero() {
			mark = "✓"
		}
		symCol := fmt.Sprintf("%s%-8s", mark, ansi.Truncate(sym, 8, ""))
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
		if !it.ReadAt.IsZero() {
			out = append(out, styleMuted.Render(symCol+catCol+title))
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
		for _, l := range wrap(s, w-2) {
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
		addWrapped(it.URL, styleMuted.Render)
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
			styleMuted.Render(fmt.Sprintf("conf %.2f · as of %s", b.Confidence, m.fmtDateTimeOr(b.CreatedAt, "—")))),
	}
	for _, l := range wrap(b.Rationale, w-2) {
		if l != "" {
			out = append(out, " "+styleText.Render(l))
		}
	}
	return out
}

// wrap word-wraps s to w cells, hard-breaking words (e.g. URLs) that are
// longer than a line.
func wrap(s string, w int) []string {
	return strings.Split(ansi.Wrap(s, max(w, 10), " -"), "\n")
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
