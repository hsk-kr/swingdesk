package ui

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/hsk-kr/swingdesk/internal/agent"
	"github.com/hsk-kr/swingdesk/internal/model"
	"github.com/hsk-kr/swingdesk/internal/refresh"
)

func typeText(t *testing.T, m Model, s string) Model {
	t.Helper()
	for _, r := range s {
		m = press(t, m, string(r))
	}
	return m
}

func TestSlashFilterNarrowsByTitleOrSymbol(t *testing.T) {
	m := newTestModel(t, 120, 40)
	m = press(t, m, "/")
	m = typeText(t, m, "nvda")
	if !m.typing || len(m.visible) != 1 || m.visible[0].Symbol != "NVDA" {
		t.Fatalf("visible = %v", visibleIDs(m))
	}
	if !strings.Contains(plain(m), "/nvda█") {
		t.Error("footer should show the query being typed")
	}
	m = press(t, m, "backspace", "backspace", "backspace", "backspace")
	m = typeText(t, m, "yields")
	if len(m.visible) != 1 || !strings.Contains(m.visible[0].Title, "yields") {
		t.Errorf("title match = %v", visibleIDs(m))
	}
	m = press(t, m, "enter")
	if m.typing || !strings.Contains(plain(m), "/yields (1)") {
		t.Error("enter should keep the query and leave typing mode")
	}
	m = press(t, m, "j") // navigation works again
	m = press(t, m, "esc")
	if m.query != "" || len(m.visible) != 10 {
		t.Errorf("esc should clear the query, visible=%d", len(m.visible))
	}
}

func TestSlashFilterEscWhileTypingAndNoMatches(t *testing.T) {
	m := newTestModel(t, 120, 40)
	m = press(t, m, "/")
	m = typeText(t, m, "zzz")
	if !strings.Contains(plain(m), "no matches for /zzz") {
		t.Error("expected no-matches state")
	}
	m = press(t, m, "esc")
	if m.typing || m.query != "" || len(m.visible) != 10 {
		t.Error("esc while typing should cancel and clear")
	}
}

func TestMarkAllRespectsQuery(t *testing.T) {
	m, store := newTestModelStore(t, 120, 40)
	m = press(t, m, "/")
	m = typeText(t, m, "TSLA")
	m = press(t, m, "enter", "a")
	if got := store.readIDs(); len(got) != 1 {
		t.Errorf("a with a query should mark only matches, read = %v", got)
	}
}

func TestCopyURL(t *testing.T) {
	m := newTestModel(t, 120, 40)
	var copied []string
	m.copyFn = func(s string) error { copied = append(copied, s); return nil }
	next, cmd := m.Update(keyMsg("c"))
	m = next.(Model)
	if cmd == nil || !strings.Contains(plain(m), "copied https://example.com/nvda-capex") {
		t.Fatalf("notice:\n%s", plain(m))
	}
	msgs := cmd().(tea.BatchMsg)
	if len(msgs) != 2 {
		t.Fatalf("want OSC 52 + tmux copy, got %d cmds", len(msgs))
	}
	for _, c := range msgs {
		if msg, ok := c().(copiedMsg); ok {
			next, _ = m.Update(msg)
		}
	}
	if !slices.Equal(copied, []string{"https://example.com/nvda-capex"}) {
		t.Errorf("copier got %v", copied)
	}
	m.copyFn = func(string) error { return errors.New("no tmux") }
	_, cmd = m.Update(keyMsg("c"))
	for _, c := range cmd().(tea.BatchMsg) {
		if msg, ok := c().(copiedMsg); ok {
			next, _ = m.Update(msg)
			if !strings.Contains(plain(next.(Model)), "copy to tmux buffer: no tmux") {
				t.Error("copier error should surface")
			}
		}
	}
}

func TestCopyWithoutURL(t *testing.T) {
	items := []model.Item{{ID: 1, Category: model.CategoryEvent, Title: "OPEC+", CreatedAt: testNow}}
	m := newModelWith(t, newFakeStore(items, nil), testInstruments(), 120, 30)
	m = press(t, m, "c")
	if !strings.Contains(plain(m), "nothing to copy") {
		t.Error("expected no-URL notice")
	}
}

func TestShowReadToggle(t *testing.T) {
	m, store := newTestModelStore(t, 120, 40)
	m = press(t, m, "r") // id 1 read
	m = press(t, m, "s")
	if len(m.visible) != 10 || !strings.Contains(plain(m), "incl. read") {
		t.Fatalf("show-read visible = %d", len(m.visible))
	}
	if !strings.Contains(plain(m), "✓NVDA") {
		t.Error("read rows should be marked")
	}
	m = press(t, m, "g", "r")
	if !strings.Contains(plain(m), "already read") {
		t.Error("r on a read row should explain")
	}
	m = press(t, m, "j", "r") // marking with show-read keeps the row, dimmed
	if len(m.visible) != 10 || len(store.readIDs()) != 2 {
		t.Errorf("visible=%d read=%v", len(m.visible), store.readIDs())
	}
	m = press(t, m, "a") // only unread rows are sent
	if len(store.readIDs()) != 10 {
		t.Errorf("read = %v", store.readIDs())
	}
	m = press(t, m, "s")
	if len(m.visible) != 0 || !strings.Contains(plain(m), "no unread items") {
		t.Errorf("hiding read: visible = %d", len(m.visible))
	}
}

func TestHelpModalOverlaysFrame(t *testing.T) {
	m := newTestModel(t, 100, 40)
	m = press(t, m, "?")
	out := plain(m)
	for _, want := range []string{"Help · j/k scroll", "Navigate", "filter by title or symbol", "copy URL (OSC 52)", "show / hide read items", "tmux attach -t swingdesk", "not financial advice", "Watchlist"} {
		if !strings.Contains(out, want) {
			t.Errorf("modal missing %q", want)
		}
	}
	assertFrame(t, m, 100, 40)
}

func TestOverlayKeepsWidth(t *testing.T) {
	base := strings.Repeat("abcdefghij\n", 5) + "abcdefghij"
	got := overlay(base, "XX\nYY", 10)
	lines := strings.Split(got, "\n")
	if lines[2] != "abcdXXghij" || lines[3] != "abcdYYghij" || lines[0] != "abcdefghij" {
		t.Errorf("overlay = %q", lines)
	}
}

func TestWaitingOnFirstRefresh(t *testing.T) {
	start := testNow
	c := clock{&start}
	calls := 0
	m, _ := newRefreshModel(t, refreshSpy{calls: &calls}, c)
	m = drive(m, loadCmd(m.store, m.loadSeq, m.currentFilter()))
	next, _ := m.Update(tickMsg(c.now())) // refresh starts, not finished
	out := plain(next.(Model))
	if !strings.Contains(out, "waiting on first refresh") || !strings.Contains(out, "tmux attach -t swingdesk") {
		t.Errorf("expected waiting state:\n%s", out)
	}
}

func TestClaudeNotLoggedInState(t *testing.T) {
	start := testNow
	c := clock{&start}
	calls := 0
	notLogged := &agent.ExitError{Job: model.JobNames, Code: 1, Stderr: "Not logged in · Please run /login"}
	spy := refreshSpy{calls: &calls, out: refresh.Outcome{Status: model.RunError, Finished: testNow, Jobs: []refresh.JobOutcome{{Job: model.JobNames, Err: notLogged}}}}
	m, _ := newRefreshModel(t, spy, c)
	m = drive(m, m.Init())
	out := plain(m)
	if !strings.Contains(out, "claude is not logged in") || !strings.Contains(out, "agents error: claude not logged in") {
		t.Errorf("expected auth state:\n%s", out)
	}
}

func exitErr(job model.Job, stderr string) error {
	return &agent.ExitError{Job: job, Code: 1, Stderr: stderr}
}

func TestIsAuthProblem(t *testing.T) {
	auth := "Invalid API key · Please run /login"
	cases := []struct {
		name string
		jobs []refresh.JobOutcome
		want bool
	}{
		{"all failed on auth", []refresh.JobOutcome{
			{Job: model.JobMarket, Err: exitErr(model.JobMarket, auth)},
			{Job: model.JobTech, Err: exitErr(model.JobTech, "OAuth token has expired")},
			{Job: model.JobNames, Err: exitErr(model.JobNames, "Not logged in")},
		}, true},
		{"one job ok, one auth", []refresh.JobOutcome{
			{Job: model.JobMarket}, {Job: model.JobTech, Err: exitErr(model.JobTech, auth)},
		}, true},
		{"mixed failure kinds", []refresh.JobOutcome{
			{Job: model.JobTech, Err: exitErr(model.JobTech, auth)}, {Job: model.JobNames, Err: agent.ErrJobTimeout},
		}, false},
		{"model prose about auth in ingest error", []refresh.JobOutcome{
			{Job: model.JobTech, Err: errors.New("claude result contains no JSON object: Okta reported an authentication outage; see https://x/login")},
		}, false},
		{"exit error unrelated", []refresh.JobOutcome{{Job: model.JobTech, Err: exitErr(model.JobTech, "fetch https://site/login failed")}}, false},
		{"no failures", []refresh.JobOutcome{{Job: model.JobTech}}, false},
	}
	for _, c := range cases {
		if got := isAuthProblem(refresh.Outcome{Jobs: c.jobs}); got != c.want {
			t.Errorf("%s = %v", c.name, got)
		}
	}
}

func TestFooterShowsDisclaimer(t *testing.T) {
	for _, w := range []int{60, 120} {
		m := newTestModel(t, w, 20)
		last := strings.Split(plain(m), "\n")[19]
		if !strings.Contains(last, "not financial advice") {
			t.Errorf("width %d footer = %q", w, last)
		}
	}
	_ = time.Second
}

func TestAllJobsFailedShowsAgentError(t *testing.T) {
	out := refresh.Outcome{Status: model.RunError, Jobs: []refresh.JobOutcome{
		{Job: model.JobMarket, Err: &agent.ExitError{Job: model.JobMarket, Code: 1}},
	}}
	st := agentStatusFor(out)
	if st.State != model.AgentError || st.Err != "market: market job exited 1" {
		t.Errorf("status = %+v", st)
	}
	if agentStatusFor(refresh.Outcome{Status: model.RunError}).Err != "all jobs failed" {
		t.Error("fallback text")
	}
}

func TestHelpOverlayWithWideCharactersKeepsFrame(t *testing.T) {
	var items []model.Item
	for i := range 10 {
		items = append(items, model.Item{ID: int64(i + 1), InstrumentID: 1, Symbol: "SKHY", Category: model.CategoryNews,
			Title: strings.Repeat("삼성전자 반도체 뉴스 ", 12), URL: "https://x", CreatedAt: testNow.Add(-time.Duration(i) * time.Minute)})
	}
	for _, sz := range [][2]int{{99, 14}, {99, 20}, {120, 40}, {61, 15}, {100, 31}} {
		m := newModelWith(t, newFakeStore(items, nil), testInstruments(), sz[0], sz[1])
		assertFrame(t, press(t, m, "?"), sz[0], sz[1])
	}
}

func TestSpliceRowStraddlingWideChars(t *testing.T) {
	row := "가나다라마바사아" // 16 cells
	for x := 0; x <= 6; x++ {
		got := spliceRow(row, "XX", x, 2, 16)
		if w := ansi.StringWidth(got); w != 16 {
			t.Errorf("x=%d width %d: %q", x, w, got)
		}
		if !strings.Contains(got, "XX") {
			t.Errorf("x=%d lost modal: %q", x, got)
		}
	}
}

func TestPasteIntoQuery(t *testing.T) {
	m := newTestModel(t, 120, 40)
	next, _ := m.Update(tea.PasteMsg{Content: "nvda\n"})
	if next.(Model).query != "" {
		t.Error("paste outside typing mode must be ignored")
	}
	m = press(t, m, "/")
	next, _ = m.Update(tea.PasteMsg{Content: "nvda\n"})
	m = next.(Model)
	if m.query != "nvda" || len(m.visible) != 1 {
		t.Errorf("query = %q visible = %d", m.query, len(m.visible))
	}
}

func TestClearingQueryKeepsSelection(t *testing.T) {
	m := newTestModel(t, 120, 40)
	m = press(t, m, "/")
	m = typeText(t, m, "TSLA")
	m = press(t, m, "enter", "esc")
	if sel, _ := m.Selected(); sel.Symbol != "TSLA" {
		t.Errorf("selection = %+v", sel)
	}
}

func TestShowReadWithUndo(t *testing.T) {
	m, store := newTestModelStore(t, 120, 40)
	m = press(t, m, "s", "r")
	if len(store.readIDs()) != 1 || len(m.visible) != 10 {
		t.Fatalf("read=%v visible=%d", store.readIDs(), len(m.visible))
	}
	m = press(t, m, "u")
	if len(store.readIDs()) != 0 || len(m.visible) != 10 || strings.Contains(plain(m), "✓") {
		t.Errorf("after undo: read=%v", store.readIDs())
	}
	if m.counts.Total != 10 {
		t.Errorf("total = %d", m.counts.Total)
	}
}
