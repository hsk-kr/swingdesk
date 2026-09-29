package refresh

import (
	"testing"
	"time"

	"github.com/hsk-kr/swingdesk/internal/config"
)

func mustHours(t *testing.T, cfg config.MarketHours) Hours {
	t.Helper()
	h, err := NewHours(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func utc(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestHoursDisabledAlwaysOpen(t *testing.T) {
	h := mustHours(t, config.MarketHours{Sessions: []config.MarketSession{config.SessionUS}})
	sat := utc("2026-09-26T03:00:00Z")
	if !h.Open(sat) || !h.NextOpen(sat).Equal(sat) {
		t.Error("disabled gate must be open")
	}
}

func TestHoursUSAndEU(t *testing.T) {
	both := mustHours(t, config.MarketHours{Enabled: true, Sessions: []config.MarketSession{config.SessionUS, config.SessionEU}})
	cases := map[string]bool{
		"2026-09-28T07:30:00Z": true,  // Mon 08:30 London (BST)
		"2026-09-28T06:30:00Z": false, // Mon 07:30 London, before open
		"2026-09-28T19:30:00Z": true,  // Mon 15:30 New York (EDT)
		"2026-09-28T20:30:00Z": false, // Mon 16:30 New York, closed; London closed too
		"2026-09-26T14:00:00Z": false, // Saturday
	}
	for ts, want := range cases {
		if got := both.Open(utc(ts)); got != want {
			t.Errorf("Open(%s) = %v", ts, got)
		}
	}
	// Friday evening → Monday London open 08:00 BST = 07:00Z.
	if got := both.NextOpen(utc("2026-09-25T21:00:00Z")); !got.Equal(utc("2026-09-28T07:00:00Z")) {
		t.Errorf("NextOpen = %v", got)
	}
}

func TestHoursPremarketAndDST(t *testing.T) {
	us := mustHours(t, config.MarketHours{Enabled: true, Premarket: true, Sessions: []config.MarketSession{config.SessionUS}})
	if !us.Open(utc("2026-09-28T08:30:00Z")) { // 04:30 New York
		t.Error("premarket should be open at 04:30 ET")
	}
	// After US DST ends (1 Nov 2026) 04:00 ET = 09:00Z.
	if got := us.NextOpen(utc("2026-11-02T08:00:00Z")); !got.Equal(utc("2026-11-02T09:00:00Z")) {
		t.Errorf("NextOpen across DST = %v", got)
	}
}

type fakeGate struct {
	open bool
	next time.Time
}

func (g fakeGate) Open(time.Time) bool          { return g.open }
func (g fakeGate) NextOpen(time.Time) time.Time { return g.next }

func TestScheduleRespectsGate(t *testing.T) {
	t0 := now
	opens := t0.Add(3 * time.Hour)
	s := NewSchedule(30*time.Minute, fakeGate{open: false, next: opens})
	s, d := s.Tick(t0)
	if d != Closed || !s.Next().Equal(opens) || s.Running() {
		t.Fatalf("closed tick = %v next %v", d, s.Next())
	}
	if _, d := s.Tick(t0.Add(time.Hour)); d != Wait {
		t.Errorf("before open = %v", d)
	}
	s, d = s.Force(t0.Add(time.Hour))
	if d != Start {
		t.Errorf("R must ignore market hours, got %v", d)
	}
}
