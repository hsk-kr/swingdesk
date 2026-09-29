package refresh

import (
	"fmt"
	"time"

	"github.com/hsk-kr/swingdesk/internal/config"
)

// Gate limits when scheduled refreshes may start.
type Gate interface {
	Open(t time.Time) bool
	NextOpen(t time.Time) time.Time
}

// window is one exchange's weekday trading window in its own timezone.
type window struct {
	loc              *time.Location
	start, end       time.Duration // offsets from local midnight
	premarketStarted time.Duration
}

// sessionSpec is the regular session and premarket start per exchange.
// Exchange holidays are not modelled.
var sessionSpec = map[config.MarketSession]struct {
	zone             string
	open, close, pre time.Duration
}{
	config.SessionUS: {"America/New_York", hm(9, 30), hm(16, 0), hm(4, 0)},
	config.SessionEU: {"Europe/London", hm(8, 0), hm(16, 30), hm(7, 0)},
}

func hm(h, m int) time.Duration { return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute }

// Hours is a Gate over exchange sessions. The zero value is always open.
type Hours struct{ windows []window }

// NewHours builds the gate for cfg; disabled config yields an always-open gate.
func NewHours(cfg config.MarketHours) (Hours, error) {
	if !cfg.Enabled {
		return Hours{}, nil
	}
	var h Hours
	for _, s := range cfg.Sessions {
		spec, ok := sessionSpec[s]
		if !ok {
			return Hours{}, fmt.Errorf("unknown market session %q", s)
		}
		loc, err := time.LoadLocation(spec.zone)
		if err != nil {
			return Hours{}, fmt.Errorf("load %s: %w", spec.zone, err)
		}
		start := spec.open
		if cfg.Premarket {
			start = spec.pre
		}
		h.windows = append(h.windows, window{loc: loc, start: start, end: spec.close})
	}
	return h, nil
}

// Open reports whether t falls inside any session (always true when disabled).
func (h Hours) Open(t time.Time) bool {
	if len(h.windows) == 0 {
		return true
	}
	for _, w := range h.windows {
		local := t.In(w.loc)
		if weekday(local) {
			since := local.Sub(midnight(local))
			if since >= w.start && since < w.end {
				return true
			}
		}
	}
	return false
}

// NextOpen is t if open, else the earliest upcoming session start.
func (h Hours) NextOpen(t time.Time) time.Time {
	if h.Open(t) {
		return t
	}
	var best time.Time
	for _, w := range h.windows {
		local := t.In(w.loc)
		for d := range 8 {
			day := midnight(local).AddDate(0, 0, d)
			if !weekday(day) {
				continue
			}
			start := addClock(day, w.start)
			if start.After(t) && (best.IsZero() || start.Before(best)) {
				best = start
				break
			}
		}
	}
	return best
}

func weekday(t time.Time) bool { return t.Weekday() != time.Saturday && t.Weekday() != time.Sunday }

func midnight(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// addClock returns the wall-clock time `off` after midnight on day (DST-safe).
func addClock(day time.Time, off time.Duration) time.Time {
	y, m, d := day.Date()
	return time.Date(y, m, d, int(off/time.Hour), int(off%time.Hour/time.Minute), 0, 0, day.Location())
}
