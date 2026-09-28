// Package watchlist parses the seed instrument list (docs/WATCHLIST.yaml).
package watchlist

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/hsk-kr/swingdesk/internal/model"
)

type entry struct {
	Symbol     string     `yaml:"symbol"`
	Name       string     `yaml:"name"`
	Kind       model.Kind `yaml:"kind"`
	CompanyTag string     `yaml:"company_tag"`
	Notes      string     `yaml:"notes"`
	Enabled    *bool      `yaml:"enabled"`
	SortOrder  int        `yaml:"sort_order"`
}

type file struct {
	Instruments []entry `yaml:"instruments"`
}

// Parse decodes and validates a watchlist document. Symbols are upper-cased
// and must be unique; enabled defaults to true.
func Parse(raw []byte) ([]model.Instrument, error) {
	var f file
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("parse watchlist: %w", err)
	}
	if len(f.Instruments) == 0 {
		return nil, errors.New("watchlist has no instruments")
	}
	seen := make(map[string]bool, len(f.Instruments))
	out := make([]model.Instrument, 0, len(f.Instruments))
	var errs []error
	for i, e := range f.Instruments {
		in, err := toInstrument(e)
		if err != nil {
			errs = append(errs, fmt.Errorf("instrument %d: %w", i, err))
			continue
		}
		if seen[in.Symbol] {
			errs = append(errs, fmt.Errorf("instrument %d: duplicate symbol %s", i, in.Symbol))
			continue
		}
		seen[in.Symbol] = true
		out = append(out, in)
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return out, nil
}

func toInstrument(e entry) (model.Instrument, error) {
	sym := strings.ToUpper(strings.TrimSpace(e.Symbol))
	if sym == "" {
		return model.Instrument{}, errors.New("symbol is required")
	}
	if strings.TrimSpace(e.Name) == "" {
		return model.Instrument{}, fmt.Errorf("%s: name is required", sym)
	}
	if !e.Kind.Valid() {
		return model.Instrument{}, fmt.Errorf("%s: invalid kind %q", sym, e.Kind)
	}
	enabled := true
	if e.Enabled != nil {
		enabled = *e.Enabled
	}
	return model.Instrument{
		Symbol:     sym,
		Name:       strings.TrimSpace(e.Name),
		Kind:       e.Kind,
		CompanyTag: strings.TrimSpace(e.CompanyTag),
		Notes:      strings.TrimSpace(e.Notes),
		Enabled:    enabled,
		SortOrder:  e.SortOrder,
	}, nil
}
