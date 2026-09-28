package ingest

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/hsk-kr/swingdesk/internal/model"
)

// checkItem returns the normalized symbol or the reason the item is skipped.
func checkItem(job model.Job, it RawItem, known map[string]model.Instrument) (string, error) {
	sym := ""
	if it.Symbol != nil {
		sym = strings.ToUpper(strings.TrimSpace(*it.Symbol))
	}
	switch {
	case sym == "" && job == model.JobNames:
		return "", errors.New("names job items need a symbol")
	case sym != "":
		if _, ok := known[sym]; !ok {
			return "", fmt.Errorf("unknown symbol %q", sym)
		}
	}
	if !it.Category.Valid() {
		return "", fmt.Errorf("invalid category %q", it.Category)
	}
	if strings.TrimSpace(it.Title) == "" {
		return "", errors.New("empty title")
	}
	link := strings.TrimSpace(it.URL)
	if link == "" {
		if at, err := parseWhen(it.EventAt); err != nil || at.IsZero() {
			return "", errors.New("no url and not a dated event")
		}
	}
	if link != "" && !isHTTPURL(link) {
		return "", fmt.Errorf("url %q is not http(s)", link)
	}
	return sym, nil
}

func isHTTPURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// checkBias returns the instrument or the reason the bias is skipped.
func checkBias(b RawBias, known map[string]model.Instrument) (model.Instrument, error) {
	sym := strings.ToUpper(strings.TrimSpace(b.Symbol))
	in, ok := known[sym]
	if !ok {
		return model.Instrument{}, fmt.Errorf("unknown symbol %q", sym)
	}
	if !b.Stance.Valid() {
		return model.Instrument{}, fmt.Errorf("invalid stance %q", b.Stance)
	}
	if b.Confidence == nil || *b.Confidence < 0 || *b.Confidence > 1 {
		return model.Instrument{}, errors.New("confidence must be within 0..1")
	}
	return in, nil
}

// parseWhen accepts RFC3339 or a bare YYYY-MM-DD (UTC midnight). Nil or
// empty yields the zero time.
func parseWhen(s *string) (time.Time, error) {
	if s == nil || strings.TrimSpace(*s) == "" {
		return time.Time{}, nil
	}
	v := strings.TrimSpace(*s)
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.DateOnly, v); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("time %q is neither RFC3339 nor YYYY-MM-DD", v)
}

// eventKind maps the optional contract field onto the closed set.
func eventKind(s *string) model.EventKind {
	if s == nil {
		return model.EventOther
	}
	k := model.EventKind(strings.ToLower(strings.TrimSpace(*s)))
	if !k.Valid() {
		return model.EventOther
	}
	return k
}
