package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/hsk-kr/swingdesk/internal/model"
	"github.com/hsk-kr/swingdesk/internal/refresh"
)

// clock is a settable time source shared by the model and the test.
type clock struct{ t *time.Time }

func (c clock) now() time.Time      { return *c.t }
func (c clock) add(d time.Duration) { *c.t = c.t.Add(d) }

type refreshSpy struct {
	calls *int
	out   refresh.Outcome
	onRun func()
}

func (s refreshSpy) fn(progress func(model.Job, error)) refresh.Outcome {
	*s.calls++
	if s.onRun != nil {
		s.onRun()
	}
	for _, j := range s.out.Jobs {
		progress(j.Job, j.Err)
	}
	return s.out
}

func newRefreshModel(t *testing.T, spy refreshSpy, c clock) (Model, fakeStore) {
	t.Helper()
	store := newFakeStore(nil, nil)
	m := New(Options{
		Store: store, Instruments: testInstruments(), Location: time.UTC,
		Now: c.now, Refresh: spy.fn, Interval: 30 * time.Minute,
	})
	m.tick = func() tea.Cmd { return nil } // tests deliver ticks by hand
	next, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	return next.(Model), store
}

func TestRefreshFiresOnStartupAndIngestedItemsAppear(t *testing.T) {
	start := testNow
	c := clock{&start}
	calls := 0
	var store fakeStore
	spy := refreshSpy{calls: &calls, out: refresh.Outcome{Status: model.RunOK, Finished: testNow,
		Jobs: []refresh.JobOutcome{{Job: model.JobNames, Inserted: 2}}}}
	spy.onRun = func() {
		store.s.items = append(store.s.items,
			model.Item{ID: 1, Category: model.CategoryNews, Title: "fresh one", CreatedAt: testNow},
			model.Item{ID: 2, Category: model.CategoryTech, Title: "fresh two", CreatedAt: testNow})
	}
	m, st := newRefreshModel(t, spy, c)
	store = st
	m = drive(m, m.Init())
	if calls != 1 {
		t.Fatalf("refresh calls = %d", calls)
	}
	out := plain(m)
	for _, want := range []string{"last refresh 11:41", "next 12:11 (in 30:00)", "agents idle", "+2 new", "refresh ok · +2 new", "fresh one"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestScheduleTicksAndOverlap(t *testing.T) {
	start := testNow
	c := clock{&start}
	calls := 0
	m, _ := newRefreshModel(t, refreshSpy{calls: &calls, out: refresh.Outcome{Status: model.RunOK, Finished: testNow}}, c)

	// Startup tick starts a refresh; hold its command (still running).
	next, _ := m.Update(tickMsg(c.now()))
	m = next.(Model)
	if !m.sched.Running() || !strings.Contains(plain(m), "agents running market,tech,names") {
		t.Fatalf("expected running state:\n%s", plain(m))
	}
	// R while running is skipped and says so.
	next, cmd := m.Update(keyMsg("R"))
	m = next.(Model)
	if cmd != nil || !strings.Contains(plain(m), "refresh skipped: previous run still running") {
		t.Errorf("R during run should skip, cmd=%v", cmd != nil)
	}
	// A due tick while running is skipped too.
	c.add(31 * time.Minute)
	next, _ = m.Update(tickMsg(c.now()))
	m = next.(Model)
	if !strings.Contains(plain(m), "refresh skipped") {
		t.Error("overlapping tick should be skipped")
	}
	next, _ = m.Update(refreshDoneMsg{out: refresh.Outcome{Status: model.RunOK, Finished: c.now()}})
	m = next.(Model)

	c.add(time.Minute)
	next, cmd = m.Update(tickMsg(c.now()))
	m = next.(Model)
	if m.sched.Running() {
		t.Error("tick before the reset deadline must wait")
	}
	_ = cmd
	next, cmd = m.Update(keyMsg("R"))
	m = drive(next.(Model), cmd)
	if calls != 1 || m.sched.Running() {
		t.Errorf("forced refresh: calls=%d running=%v", calls, m.sched.Running())
	}
	if !m.sched.Next().Equal(c.now().Add(30 * time.Minute)) {
		t.Errorf("R should reset the timer, next = %v", m.sched.Next())
	}
}

func TestRefreshErrorShowsInHeader(t *testing.T) {
	start := testNow
	c := clock{&start}
	calls := 0
	spy := refreshSpy{calls: &calls, out: refresh.Outcome{Status: model.RunError, Err: errors.New("tmux not found"), Finished: testNow}}
	m, _ := newRefreshModel(t, spy, c)
	m = drive(m, m.Init())
	out := plain(m)
	if !strings.Contains(out, "agents error: tmux not found") || !strings.Contains(out, "refresh failed: tmux not found") {
		t.Errorf("render:\n%s", out)
	}
}

func TestPartialRefreshNotice(t *testing.T) {
	msg, isErr := refreshNotice(refresh.Outcome{Status: model.RunPartial, Jobs: []refresh.JobOutcome{
		{Job: model.JobMarket, Inserted: 3}, {Job: model.JobTech, Err: errors.New("job timed out")},
	}})
	if !isErr || msg != "refresh partial (+3 new) · tech: job timed out" {
		t.Errorf("notice = %q %v", msg, isErr)
	}
}

func TestRWithoutRefresherExplains(t *testing.T) {
	m := newTestModel(t, 120, 30)
	m = press(t, m, "R")
	if !strings.Contains(plain(m), "refresh is not configured") {
		t.Error("expected explanation")
	}
}

func TestJobProgressShrinksRunningList(t *testing.T) {
	start := testNow
	c := clock{&start}
	calls := 0
	m, _ := newRefreshModel(t, refreshSpy{calls: &calls}, c)
	next, _ := m.Update(tickMsg(c.now()))
	m = next.(Model)
	ch := make(chan tea.Msg)
	next, cmd := m.Update(jobDoneMsg{job: model.JobTech, ch: ch})
	m = next.(Model)
	if !strings.Contains(plain(m), "agents running market,names") {
		t.Errorf("header:\n%s", strings.Split(plain(m), "\n")[0])
	}
	if cmd == nil {
		t.Fatal("job progress must re-arm the channel read")
	}
	close(ch)
	if msg := cmd(); msg != nil {
		t.Errorf("closed channel should yield nil, got %T", msg)
	}
}

func TestRunRefreshStreamsProgressThenDone(t *testing.T) {
	fn := func(progress func(model.Job, error)) refresh.Outcome {
		progress(model.JobMarket, nil)
		progress(model.JobNames, errors.New("x"))
		return refresh.Outcome{Status: model.RunPartial}
	}
	msg := runRefresh(fn)()
	var got []string
	for msg != nil {
		switch v := msg.(type) {
		case jobDoneMsg:
			got = append(got, string(v.job))
			msg = next(v.ch)()
		case refreshDoneMsg:
			got = append(got, "done:"+string(v.out.Status))
			msg = nil
		default:
			t.Fatalf("unexpected %T", msg)
		}
	}
	if strings.Join(got, ",") != "market,names,done:partial" {
		t.Errorf("stream = %v", got)
	}
}

func TestCountdownAndFlashExpire(t *testing.T) {
	start := testNow
	c := clock{&start}
	calls := 0
	spy := refreshSpy{calls: &calls, out: refresh.Outcome{Status: model.RunOK, Finished: testNow,
		Jobs: []refresh.JobOutcome{{Job: model.JobNames, Inserted: 4}}}}
	m, _ := newRefreshModel(t, spy, c)
	m = drive(m, m.Init())
	if !strings.Contains(plain(m), "+4 new") {
		t.Fatal("flash missing")
	}
	c.add(95 * time.Second)
	next, _ := m.Update(tickMsg(c.now()))
	m = next.(Model)
	head := strings.Split(plain(m), "\n")[0]
	if strings.Contains(head, "+4 new") {
		t.Error("flash should expire")
	}
	if !strings.Contains(head, "(in 28:25)") {
		t.Errorf("countdown missing: %q", head)
	}
}

func TestFmtCountdown(t *testing.T) {
	cases := map[time.Duration]string{
		0: "0:00", 59 * time.Second: "0:59", 30 * time.Minute: "30:00",
		90*time.Minute + 5*time.Second: "1:30:05",
	}
	for d, want := range cases {
		if got := fmtCountdown(d); got != want {
			t.Errorf("fmtCountdown(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestHelpMentionsTmuxAttach(t *testing.T) {
	m := New(Options{Store: newFakeStore(nil, nil), Instruments: testInstruments(), TmuxSession: "desk2"})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 50})
	m = press(t, next.(Model), "?")
	if !strings.Contains(plain(m), "tmux attach -t desk2") {
		t.Error("help should mention tmux attach with the session name")
	}
}
