// Package refresh runs refreshes (agents + ingest) and decides when.
package refresh

import "time"

// Schedule is the in-process refresh timer. It is a value type: every method
// returns an updated copy.
type Schedule struct {
	interval time.Duration
	next     time.Time // zero until the first start
	running  bool
}

// NewSchedule returns a schedule that fires immediately on the first Tick.
func NewSchedule(interval time.Duration) Schedule {
	return Schedule{interval: interval}
}

// Decision is what a Tick or Force asks the caller to do.
type Decision int

const (
	Wait    Decision = iota // nothing due
	Start                   // start a refresh now
	Skipped                 // due, but a run is still going; timer was reset
)

// Tick is called periodically. The first tick always starts a refresh.
func (s Schedule) Tick(now time.Time) (Schedule, Decision) {
	if !s.next.IsZero() && now.Before(s.next) {
		return s, Wait
	}
	return s.begin(now)
}

// Force starts a refresh now (the R key) and resets the timer.
func (s Schedule) Force(now time.Time) (Schedule, Decision) {
	return s.begin(now)
}

func (s Schedule) begin(now time.Time) (Schedule, Decision) {
	s.next = now.Add(s.interval)
	if s.running {
		return s, Skipped
	}
	s.running = true
	return s, Start
}

// Done marks the in-flight refresh as finished.
func (s Schedule) Done() Schedule {
	s.running = false
	return s
}

// Running reports whether a refresh is in flight.
func (s Schedule) Running() bool { return s.running }

// Next is when the next refresh is due (zero before the first).
func (s Schedule) Next() time.Time { return s.next }
