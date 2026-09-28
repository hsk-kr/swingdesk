package ui

import (
	"time"

	"github.com/hsk-kr/swingdesk/internal/model"
)

type fixture struct {
	symbol   string
	category model.Category
	title    string
	summary  string
	source   string
	url      string
	ageMin   int
}

// fixtureRows are placeholder inbox items so the chrome is reviewable before
// the ingest pipeline exists. Removed once the inbox reads from SQLite.
var fixtureRows = []fixture{
	{"NVDA", model.CategoryNews, "Hyperscaler capex guide lifts AI accelerator orders",
		"Two cloud providers raised FY capex guidance, citing accelerator supply. Near-term read-through is positive for NVDA into next week's supplier conference.",
		"Reuters", "https://example.com/nvda-capex", 12},
	{"MSFT", model.CategoryEvent, "Ignite keynote scheduled for 21 Oct",
		"Microsoft confirmed the Ignite keynote date. Copilot pricing updates are expected; historically a low-volatility event.",
		"Microsoft IR", "https://example.com/msft-ignite", 25},
	{"META", model.CategoryOpinion, "Street trims targets on ad-load concerns",
		"Three brokers cut price targets citing slower ad-load growth. Ratings unchanged. Tone is cautious rather than bearish.",
		"Bloomberg", "https://example.com/meta-targets", 40},
	{"CRUDE", model.CategoryMarket, "Inventory build larger than expected",
		"Weekly crude inventories rose more than consensus. Front-month weakness into the 16 Oct expiry is the swing-relevant angle.",
		"EIA", "https://example.com/eia-weekly", 55},
	{"TECH100", model.CategoryMarket, "Index futures flat ahead of CPI",
		"Tech-heavy futures are flat into tomorrow's CPI print. Rate-sensitive megacaps likely to set direction.",
		"CNBC", "https://example.com/futures-cpi", 70},
	{"", model.CategoryTech, "HBM supply tightness persists into Q1",
		"Supplier commentary points to high-bandwidth memory staying sold out. Relevant for SKHY and GPU vendors.",
		"The Information", "https://example.com/hbm-supply", 90},
	{"AAPL", model.CategoryValuation, "Shares trade at a premium to 5y average P/E",
		"Forward multiple sits above its five-year average after the recent run. Limited near-term catalyst until earnings.",
		"FactSet", "https://example.com/aapl-pe", 120},
	{"TSLA", model.CategoryNews, "Q3 delivery estimate consensus drifts lower",
		"Sell-side delivery estimates slipped ahead of the quarterly print. Delivery day is a known volatility event.",
		"Electrek", "https://example.com/tsla-deliveries", 150},
	{"", model.CategoryMarket, "Treasury yields ease after weak jobs data",
		"The 10-year yield fell after softer payrolls. Supports long-duration tech in the short run.",
		"WSJ", "https://example.com/yields", 180},
	{"SPCX", model.CategoryNews, "Launch cadence update from company event",
		"Private company; CFD price reacts to headlines only. Treat as headline-driven, low information.",
		"SpaceNews", "https://example.com/spacex-cadence", 240},
}

// FixtureItems returns placeholder unread items, newest first, resolved
// against the given instruments. Items for unknown symbols become general
// tape items.
func FixtureItems(instruments []model.Instrument, now time.Time) []model.Item {
	bySymbol := make(map[string]model.Instrument, len(instruments))
	for _, in := range instruments {
		bySymbol[in.Symbol] = in
	}
	out := make([]model.Item, 0, len(fixtureRows))
	for i, f := range fixtureRows {
		it := model.Item{
			ID:          int64(i + 1),
			Category:    f.category,
			Title:       f.title,
			Summary:     f.summary,
			Source:      f.source,
			URL:         f.url,
			PublishedAt: now.Add(-time.Duration(f.ageMin+5) * time.Minute),
			CreatedAt:   now.Add(-time.Duration(f.ageMin) * time.Minute),
		}
		if in, ok := bySymbol[f.symbol]; ok {
			it.InstrumentID = in.ID
			it.Symbol = in.Symbol
		}
		out = append(out, it)
	}
	return out
}

// FixtureBiases returns placeholder stances for a few instruments.
func FixtureBiases(instruments []model.Instrument, now time.Time) map[int64]model.Bias {
	calls := map[string]model.Bias{
		"NVDA":  {Stance: model.StanceLong, Confidence: 0.62, Rationale: "Capex read-through is fresh; supplier conference is a near-term catalyst."},
		"META":  {Stance: model.StanceNone, Confidence: 0.40, Rationale: "Target cuts are incremental. No clean 3-10 day edge."},
		"CRUDE": {Stance: model.StanceShort, Confidence: 0.55, Rationale: "Inventory build into expiry pressures the front month."},
	}
	out := map[int64]model.Bias{}
	for _, in := range instruments {
		if b, ok := calls[in.Symbol]; ok {
			b.InstrumentID = in.ID
			b.CreatedAt = now
			out[in.ID] = b
		}
	}
	return out
}
