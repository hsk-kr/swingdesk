package ui

import "github.com/hsk-kr/swingdesk/internal/model"

// Filter is one entry in the left pane.
type Filter struct {
	Label string
	Query model.ItemFilter
}

// buildFilters returns All, Tech, Market, then each enabled instrument.
func buildFilters(instruments []model.Instrument) []Filter {
	out := []Filter{
		{Label: "All"},
		{Label: "Tech", Query: model.ItemFilter{Category: model.CategoryTech}},
		{Label: "Market", Query: model.ItemFilter{Category: model.CategoryMarket}},
	}
	for _, in := range instruments {
		if !in.Enabled {
			continue
		}
		out = append(out, Filter{Label: in.Symbol, Query: model.ItemFilter{InstrumentID: in.ID}})
	}
	return out
}
