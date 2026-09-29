// Package refresh runs refreshes (agents + ingest) and decides when.
package refresh

import "time"

// Schedule is the in-process refresh timer. It is a value type: every method
// returns an updated copy.
type Schedule struct {
	interval time.Duration
	gate     Gate      // nil = always open
	next     time.Time // zero until the first start
	running  bool
}

// NewSchedule returns a schedule that fires on the first Tick if gate is
// open (nil gate: always).
func NewSchedule(interval time.Duration, gate Gate) Schedule {
	return Schedule{interval: interval, gate: gate}
}

// Decision is what a Tick or Force asks the caller to do.
type Decision int

const (
	Wait    Decision = iota // nothing due
	Start                   // start a refresh now
	Skipped                 // due, but a run is still going; timer was reset
	Closed                  // due, but outside market hours; next = next open
)

// Tick is called periodically. The first tick starts a refresh unless the
// gate is closed, in which case the next attempt is at the next open.
func (s Schedule) Tick(now time.Time) (Schedule, Decision) {
	if !s.next.IsZero() && now.Before(s.next) {
		return s, Wait
	}
	if s.gate != nil && !s.gate.Open(now) {
		s.next = s.gate.NextOpen(now)
		return s, Closed
	}
	return s.begin(now)
}

// Force starts a refresh now (the R key), ignoring market hours, and resets
// the timer.
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
