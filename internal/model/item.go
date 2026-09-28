package model

import (
	"slices"
	"time"
)

// Category is an inbox item category.
type Category string

const (
	CategoryTech      Category = "tech"
	CategoryMarket    Category = "market"
	CategoryNews      Category = "news"
	CategoryOpinion   Category = "opinion"
	CategoryEvent     Category = "event"
	CategoryValuation Category = "valuation"
	CategoryOther     Category = "other"
)

var categories = [...]Category{
	CategoryTech, CategoryMarket, CategoryNews, CategoryOpinion,
	CategoryEvent, CategoryValuation, CategoryOther,
}

// Valid reports whether c is in the closed set.
func (c Category) Valid() bool { return slices.Contains(categories[:], c) }

// Stance is a swing bias.
type Stance string

const (
	StanceLong  Stance = "long"
	StanceShort Stance = "short"
	StanceNone  Stance = "none"
)

var stances = [...]Stance{StanceLong, StanceShort, StanceNone}

// Valid reports whether s is in the closed set.
func (s Stance) Valid() bool { return slices.Contains(stances[:], s) }

// Item is one inbox entry. InstrumentID is 0 and Symbol empty for general
// tape items that are not tied to an instrument.
type Item struct {
	ID           int64
	InstrumentID int64
	Symbol       string
	Category     Category
	Title        string
	Summary      string
	Body         string
	Source       string
	URL          string
	PublishedAt  time.Time // zero when unknown
	CreatedAt    time.Time
	ReadAt       time.Time // zero when unread
}

// Bias is the latest stance for an instrument.
type Bias struct {
	InstrumentID int64
	Stance       Stance
	Confidence   float64
	Rationale    string
	CreatedAt    time.Time
}
