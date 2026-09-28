package ui

import "github.com/hsk-kr/swingdesk/internal/model"

type filterKind int

const (
	filterAll filterKind = iota
	filterCategory
	filterInstrument
)

// Filter is one entry in the left pane.
type Filter struct {
	kind         filterKind
	category     model.Category
	instrumentID int64
	Label        string
}

// Match reports whether it belongs under f.
func (f Filter) Match(it model.Item) bool {
	switch f.kind {
	case filterCategory:
		return it.Category == f.category
	case filterInstrument:
		return it.InstrumentID == f.instrumentID
	default:
		return true
	}
}

// buildFilters returns All, Tech, Market, then each enabled instrument.
func buildFilters(instruments []model.Instrument) []Filter {
	out := []Filter{
		{kind: filterAll, Label: "All"},
		{kind: filterCategory, category: model.CategoryTech, Label: "Tech"},
		{kind: filterCategory, category: model.CategoryMarket, Label: "Market"},
	}
	for _, in := range instruments {
		if !in.Enabled {
			continue
		}
		out = append(out, Filter{kind: filterInstrument, instrumentID: in.ID, Label: in.Symbol})
	}
	return out
}

// applyFilter returns the items matching f, preserving order.
func applyFilter(items []model.Item, f Filter) []model.Item {
	out := make([]model.Item, 0, len(items))
	for _, it := range items {
		if f.Match(it) {
			out = append(out, it)
		}
	}
	return out
}

// countMatches returns the number of items under f (the unread badge).
func countMatches(items []model.Item, f Filter) int {
	n := 0
	for _, it := range items {
		if f.Match(it) {
			n++
		}
	}
	return n
}
