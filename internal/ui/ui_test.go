package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/hsk-kr/swingdesk/internal/model"
)

var testNow = time.Date(2026, 9, 27, 11, 41, 0, 0, time.UTC)

func testInstruments() []model.Instrument {
	syms := []string{"NVDA", "MSFT", "META", "CRUDE", "TECH100", "AAPL", "TSLA", "SPCX"}
	out := make([]model.Instrument, 0, len(syms)+1)
	for i, s := range syms {
		out = append(out, model.Instrument{ID: int64(i + 1), Symbol: s, Name: s, Kind: model.KindEquity, Enabled: true})
	}
	return append(out, model.Instrument{ID: 99, Symbol: "OFF", Name: "Disabled", Kind: model.KindOther})
}

func newTestModel(t *testing.T, w, h int) Model {
	t.Helper()
	ins := testInstruments()
	m := New(Options{
		Instruments: ins,
		Items:       FixtureItems(ins, testNow),
		Biases:      FixtureBiases(ins, testNow),
		Location:    time.UTC,
	})
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return next.(Model)
}

func press(t *testing.T, m Model, keys ...string) Model {
	t.Helper()
	for _, k := range keys {
		next, _ := m.Update(keyMsg(k))
		m = next.(Model)
	}
	return m
}

func keyMsg(k string) tea.KeyPressMsg {
	switch k {
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	}
	r := []rune(k)[0]
	return tea.KeyPressMsg{Code: r, Text: k}
}

func plain(m Model) string { return ansi.Strip(m.render()) }

func TestFixturesResolveInstruments(t *testing.T) {
	items := FixtureItems(testInstruments(), testNow)
	if len(items) < 8 || len(items) > 10 {
		t.Fatalf("fixture count = %d, want 8-10", len(items))
	}
	for i := 1; i < len(items); i++ {
		if items[i].CreatedAt.After(items[i-1].CreatedAt) {
			t.Fatal("fixtures must be newest first")
		}
	}
	if items[0].Symbol != "NVDA" || items[0].InstrumentID != 1 {
		t.Errorf("first = %+v", items[0])
	}
	for _, it := range items {
		if !it.Category.Valid() {
			t.Errorf("invalid category %q", it.Category)
		}
	}
}

func TestStyleMapsCoverClosedSets(t *testing.T) {
	for _, c := range model.Categories() {
		if _, ok := categoryColors[c]; !ok {
			t.Errorf("no color for category %q", c)
		}
	}
	for _, s := range model.Stances() {
		if _, ok := stanceColors[s]; !ok {
			t.Errorf("no color for stance %q", s)
		}
	}
}

func TestRenderLayout(t *testing.T) {
	m := newTestModel(t, 120, 40)
	out := plain(m)
	lines := strings.Split(out, "\n")
	if len(lines) != 40 {
		t.Errorf("rendered %d lines, want 40", len(lines))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 120 {
			t.Errorf("line %d width %d, want 120: %q", i, w, l)
		}
	}
	for _, want := range []string{"swingdesk", "unread 10", "last refresh never", "agents idle",
		"Watchlist", "Inbox · All · unread 10", "Detail", "NVDA", "Hyperscaler capex",
		"stance:", "long", "conf 0.62", "q quit"} {
		if !strings.Contains(out, want) {
			t.Errorf("render missing %q", want)
		}
	}
	if strings.Contains(out, "OFF") {
		t.Error("disabled instrument should not appear in the left pane")
	}
}

func TestTooSmallAndLoading(t *testing.T) {
	m := New(Options{Instruments: testInstruments()})
	if plain(m) != "loading…" {
		t.Errorf("before size: %q", plain(m))
	}
	small := newTestModel(t, 40, 10)
	if !strings.Contains(plain(small), "terminal too small") {
		t.Error("expected too-small message")
	}
}

func TestInboxNavigation(t *testing.T) {
	m := newTestModel(t, 120, 40)
	if m.focus != paneInbox {
		t.Fatalf("initial focus = %v", m.focus)
	}
	m = press(t, m, "j", "j", "down")
	if sel, _ := m.Selected(); sel.ID != 4 {
		t.Errorf("after 3 downs selected %d", sel.ID)
	}
	m = press(t, m, "k")
	if sel, _ := m.Selected(); sel.ID != 3 {
		t.Errorf("after up selected %d", sel.ID)
	}
	m = press(t, m, "G")
	if sel, _ := m.Selected(); sel.ID != 10 {
		t.Errorf("G selected %d", sel.ID)
	}
	m = press(t, m, "j")
	if sel, _ := m.Selected(); sel.ID != 10 {
		t.Error("cursor should clamp at bottom")
	}
	m = press(t, m, "g")
	if sel, _ := m.Selected(); sel.ID != 1 {
		t.Errorf("g selected %d", sel.ID)
	}
}

func TestPaneSwitching(t *testing.T) {
	m := newTestModel(t, 120, 40)
	m = press(t, m, "h")
	if m.focus != paneFilters {
		t.Errorf("h -> %v", m.focus)
	}
	m = press(t, m, "h")
	if m.focus != paneFilters {
		t.Error("h should clamp at left")
	}
	m = press(t, m, "l", "l", "l")
	if m.focus != paneDetail {
		t.Errorf("l -> %v", m.focus)
	}
	m = press(t, m, "tab")
	if m.focus != paneFilters {
		t.Errorf("tab should wrap to filters, got %v", m.focus)
	}
	m = press(t, m, "shift+tab")
	if m.focus != paneDetail {
		t.Errorf("shift+tab should wrap to detail, got %v", m.focus)
	}
	m = press(t, m, "esc")
	if m.focus != paneInbox {
		t.Errorf("esc -> %v", m.focus)
	}
	m = press(t, m, "enter")
	if m.focus != paneDetail {
		t.Errorf("enter -> %v", m.focus)
	}
}

func TestFilterPane(t *testing.T) {
	m := newTestModel(t, 120, 40)
	m = press(t, m, "h", "j") // Tech
	if got := len(m.visible); got != 1 || m.visible[0].Category != model.CategoryTech {
		t.Errorf("Tech filter visible = %d", got)
	}
	m = press(t, m, "j") // Market
	for _, it := range m.visible {
		if it.Category != model.CategoryMarket {
			t.Errorf("Market filter leaked %q", it.Category)
		}
	}
	if len(m.visible) != 3 {
		t.Errorf("Market visible = %d", len(m.visible))
	}
	m = press(t, m, "j") // NVDA
	if len(m.visible) != 1 || m.visible[0].Symbol != "NVDA" {
		t.Errorf("NVDA filter = %+v", m.visible)
	}
	out := plain(m)
	if !strings.Contains(out, "Inbox · NVDA · unread 1") {
		t.Error("inbox title should reflect filter")
	}
	m = press(t, m, "G") // last enabled instrument: SPCX
	if m.filters[m.filterCursor].Label != "SPCX" {
		t.Errorf("last filter = %s", m.filters[m.filterCursor].Label)
	}
}

func TestEmptyFilterShowsEmptyState(t *testing.T) {
	ins := testInstruments()
	m := New(Options{Instruments: ins, Location: time.UTC})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	out := plain(next.(Model))
	if !strings.Contains(out, "no unread items") || !strings.Contains(out, "nothing selected") {
		t.Error("expected empty states")
	}
}

func TestDetailScrollIsClamped(t *testing.T) {
	m := newTestModel(t, 80, 16)
	m = press(t, m, "l")
	for range 50 {
		m = press(t, m, "j")
	}
	if m.detailScroll != m.maxDetailScroll() {
		t.Errorf("scroll = %d, max %d", m.detailScroll, m.maxDetailScroll())
	}
	m = press(t, m, "k")
	if m.detailScroll != max(m.maxDetailScroll()-1, 0) {
		t.Error("one k should scroll back one line")
	}
}

func TestHelpToggleAndQuit(t *testing.T) {
	m := newTestModel(t, 120, 40)
	m = press(t, m, "?")
	if !m.showHelp || !strings.Contains(plain(m), "change pane") {
		t.Fatal("help not shown")
	}
	m = press(t, m, "j")
	if sel, _ := m.Selected(); sel.ID != 1 {
		t.Error("keys other than close/quit should be ignored while help is open")
	}
	m = press(t, m, "?")
	if m.showHelp {
		t.Error("? should close help")
	}
	for _, k := range []string{"q", "ctrl+c"} {
		_, cmd := m.Update(keyMsg(k))
		if cmd == nil {
			t.Fatalf("%s returned nil cmd", k)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Errorf("%s did not quit", k)
		}
	}
}

func TestViewSetsAltScreen(t *testing.T) {
	v := newTestModel(t, 100, 30).View()
	if !v.AltScreen {
		t.Error("AltScreen should be on")
	}
}

func TestScrollWindow(t *testing.T) {
	cases := []struct{ cursor, n, h, start, end int }{
		{0, 10, 5, 0, 5},
		{4, 10, 5, 0, 5},
		{5, 10, 5, 1, 6},
		{9, 10, 5, 5, 10},
		{0, 3, 5, 0, 3},
		{0, 0, 5, 0, 0},
	}
	for _, c := range cases {
		s, e := scrollWindow(c.cursor, c.n, c.h)
		if s != c.start || e != c.end {
			t.Errorf("scrollWindow(%d,%d,%d) = %d,%d want %d,%d", c.cursor, c.n, c.h, s, e, c.start, c.end)
		}
	}
}

func assertFrame(t *testing.T, m Model, w, h int) {
	t.Helper()
	lines := strings.Split(plain(m), "\n")
	if len(lines) != h {
		t.Errorf("%dx%d: %d lines", w, h, len(lines))
	}
	for i, l := range lines {
		if got := ansi.StringWidth(l); got != w {
			t.Errorf("%dx%d: line %d width %d: %q", w, h, i, got, l)
		}
	}
}

func TestFrameExactAtManySizes(t *testing.T) {
	for _, sz := range [][2]int{{60, 14}, {61, 15}, {73, 17}, {99, 31}, {60, 40}, {150, 14}, {200, 60}} {
		m := newTestModel(t, sz[0], sz[1])
		assertFrame(t, m, sz[0], sz[1])
		assertFrame(t, press(t, m, "l"), sz[0], sz[1])
		assertFrame(t, press(t, m, "?"), sz[0], sz[1])
	}
}

func TestWideCharactersFit(t *testing.T) {
	ins := testInstruments()
	items := []model.Item{{
		ID: 1, InstrumentID: 1, Symbol: "NVDA", Category: model.CategoryNews,
		Title:   "エヌビディア 株価 急騰 🚀🚀 データセンター需要が過去最高を更新し続ける",
		Summary: "韓国 SK하이닉스 HBM 공급 부족 지속 🚀 " + strings.Repeat("広い文字 ", 30),
		URL:     "https://example.com/" + strings.Repeat("very-long-path-segment-", 12),
	}}
	for _, sz := range [][2]int{{60, 14}, {87, 20}, {120, 40}} {
		m := New(Options{Instruments: ins, Items: items, Location: time.UTC})
		next, _ := m.Update(tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
		assertFrame(t, next.(Model), sz[0], sz[1])
	}
}

func TestLongURLIsWrappedNotTruncated(t *testing.T) {
	url := "https://example.com/" + strings.Repeat("abcdefghij", 15)
	lines := wrap(url, 40)
	if len(lines) < 4 || strings.Join(lines, "") != url {
		t.Errorf("wrap lost characters: %q", lines)
	}
	for _, l := range lines {
		if ansi.StringWidth(l) > 40 {
			t.Errorf("line too wide: %q", l)
		}
	}
}

func TestHelpScrollsIndependently(t *testing.T) {
	m := newTestModel(t, 60, 14)
	m = press(t, m, "l", "j", "j", "j")
	m = press(t, m, "?")
	if m.helpScroll != 0 || !strings.Contains(plain(m), "Keys") {
		t.Fatal("help must open at the top regardless of detail scroll")
	}
	if m.maxHelpScroll() == 0 {
		t.Fatal("help should overflow at 60x14")
	}
	m = press(t, m, "G")
	if !strings.Contains(plain(m), "q          quit") {
		t.Error("G should reveal the end of help")
	}
	m = press(t, m, "g")
	if m.helpScroll != 0 {
		t.Error("g should return to top of help")
	}
	m = press(t, m, "j")
	if m.helpScroll != 1 {
		t.Error("j should scroll help")
	}
}

func TestNarrowHeaderKeepsAgentsAndFooterKeepsHelpQuit(t *testing.T) {
	m := newTestModel(t, 60, 14)
	lines := strings.Split(plain(m), "\n")
	if !strings.Contains(lines[0], "agents idle") || !strings.Contains(lines[0], "unread 10") {
		t.Errorf("header = %q", lines[0])
	}
	last := lines[len(lines)-1]
	if !strings.Contains(last, "? help") || !strings.Contains(last, "q quit") {
		t.Errorf("footer = %q", last)
	}
}

func TestHeaderShowsAgentStatus(t *testing.T) {
	m := New(Options{
		Instruments: testInstruments(),
		Location:    time.UTC,
		Status: Status{
			LastRefresh: testNow,
			NextRefresh: testNow.Add(30 * time.Minute),
			Agents:      model.AgentStatus{State: model.AgentError, Err: "tmux not found"},
		},
	})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 20})
	head := strings.Split(plain(next.(Model)), "\n")[0]
	for _, want := range []string{"last refresh 11:41", "next refresh 12:11", "agents error: tmux not found"} {
		if !strings.Contains(head, want) {
			t.Errorf("header missing %q: %q", want, head)
		}
	}
}

func TestJoinFittingDropsByPriority(t *testing.T) {
	parts := []part{{"aaaa", 0}, {"bbbb", 2}, {"cccc", 1}}
	if got := joinFitting(parts, " ", 100); got != "aaaa bbbb cccc" {
		t.Errorf("fits: %q", got)
	}
	if got := joinFitting(parts, " ", 10); got != "aaaa cccc" {
		t.Errorf("drop b: %q", got)
	}
	if got := joinFitting(parts, " ", 5); got != "aaaa" {
		t.Errorf("drop to one: %q", got)
	}
	if len(parts) != 3 {
		t.Error("input must not be mutated")
	}
}
