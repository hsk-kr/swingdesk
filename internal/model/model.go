// Package model holds the closed domain vocabularies shared by db, ingest and ui.
package model

import "slices"

// Kind is an instrument kind.
type Kind string

const (
	KindEquity    Kind = "equity"
	KindCFD       Kind = "cfd"
	KindIndex     Kind = "index"
	KindCommodity Kind = "commodity"
	KindOther     Kind = "other"
)

var kinds = [...]Kind{KindEquity, KindCFD, KindIndex, KindCommodity, KindOther}

// Valid reports whether k is in the closed set.
func (k Kind) Valid() bool { return slices.Contains(kinds[:], k) }

// Instrument is a watchlist row.
type Instrument struct {
	ID         int64
	Symbol     string
	Name       string
	Kind       Kind
	CompanyTag string
	Notes      string
	Enabled    bool
	SortOrder  int
}
