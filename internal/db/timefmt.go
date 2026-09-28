package db

import (
	"fmt"
	"time"
)

// timeLayout is fixed-width UTC so TEXT columns sort chronologically.
// (RFC3339Nano trims trailing zeros, which breaks lexical ordering.)
const timeLayout = "2006-01-02T15:04:05.000000000Z"

func formatTime(t time.Time) string { return t.UTC().Format(timeLayout) }

// nullTime formats t, or returns NULL for the zero time.
func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return formatTime(t)
}

// parseTime accepts timeLayout or any RFC3339 value; empty yields zero.
func parseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse time %q: %w", s, err)
	}
	return t, nil
}
