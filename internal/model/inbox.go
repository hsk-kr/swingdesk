package model

// ItemFilter narrows the inbox. Zero fields mean "any"; read items are
// excluded unless IncludeRead is set (the `s` toggle).
type ItemFilter struct {
	InstrumentID int64
	Category     Category
	IncludeRead  bool
}

// UnreadCounts are the badge numbers for the left pane.
type UnreadCounts struct {
	Total        int
	ByCategory   map[Category]int
	ByInstrument map[int64]int
}

// Count returns the unread count for f. Only single-dimension filters (all,
// one category, or one instrument) are supported, matching the left pane.
func (c UnreadCounts) Count(f ItemFilter) int {
	switch {
	case f.InstrumentID != 0:
		return c.ByInstrument[f.InstrumentID]
	case f.Category != "":
		return c.ByCategory[f.Category]
	default:
		return c.Total
	}
}
