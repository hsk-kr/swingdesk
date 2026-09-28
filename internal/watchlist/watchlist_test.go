package watchlist

import (
	"testing"

	"github.com/hsk-kr/swingdesk"
	"github.com/hsk-kr/swingdesk/internal/model"
)

func TestParseEmbeddedSeed(t *testing.T) {
	got, err := Parse(swingdesk.WatchlistYAML)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(got) != 14 {
		t.Fatalf("len = %d, want 14", len(got))
	}
	bySym := map[string]model.Instrument{}
	for _, in := range got {
		bySym[in.Symbol] = in
		if !in.Enabled {
			t.Errorf("%s should default to enabled", in.Symbol)
		}
	}
	if bySym["SPCX"].Kind != model.KindCFD || bySym["SPCX"].Notes == "" {
		t.Errorf("SPCX lost kind/notes: %+v", bySym["SPCX"])
	}
	if bySym["CRUDE"].Kind != model.KindCommodity || bySym["TECH100"].Kind != model.KindIndex {
		t.Errorf("CRUDE/TECH100 kinds wrong")
	}
	if bySym["GOOG"].CompanyTag != "alphabet" || bySym["GOOGL"].CompanyTag != "alphabet" {
		t.Errorf("alphabet tag missing")
	}
}

func TestParseRejects(t *testing.T) {
	cases := map[string]string{
		"bad kind":      "instruments:\n  - {symbol: X, name: X, kind: stock}\n",
		"no symbol":     "instruments:\n  - {name: X, kind: equity}\n",
		"no name":       "instruments:\n  - {symbol: X, kind: equity}\n",
		"dup symbol":    "instruments:\n  - {symbol: X, name: X, kind: equity}\n  - {symbol: x, name: Y, kind: equity}\n",
		"unknown field": "instruments:\n  - {symbol: X, name: X, kind: equity, tikcer: X}\n",
		"empty list":    "instruments: []\n",
		"bad yaml":      "instruments: [\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(body)); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestParseNormalizesSymbol(t *testing.T) {
	got, err := Parse([]byte("instruments:\n  - {symbol: ' nvda ', name: Nvidia, kind: equity, enabled: false}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Symbol != "NVDA" || got[0].Enabled {
		t.Errorf("got %+v", got[0])
	}
}
