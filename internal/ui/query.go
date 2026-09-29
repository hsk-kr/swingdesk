package ui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/hsk-kr/swingdesk/internal/model"
)

// handleQueryKey edits the `/` filter: typing narrows live, enter keeps the
// query, esc clears it.
func (m Model) handleQueryKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Back):
		m.typing = false
		return m.setQuery(""), nil
	case msg.String() == "enter":
		m.typing = false
		return m, nil
	case msg.String() == "backspace":
		r := []rune(m.query)
		if len(r) == 0 {
			m.typing = false
			return m, nil
		}
		return m.setQuery(string(r[:len(r)-1])), nil
	case msg.String() == "ctrl+c":
		return m, tea.Quit
	case msg.Text != "":
		return m.setQuery(m.query + msg.Text), nil
	}
	return m, nil
}

// setQuery re-derives the visible rows for q.
func (m Model) setQuery(q string) Model {
	m.query = q
	m.itemCursor = 0
	m.detailScroll = 0
	return m.setItems(m.items)
}

// matchQuery keeps items whose title or symbol contains q (case-insensitive).
func matchQuery(items []model.Item, q string) []model.Item {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return items
	}
	out := make([]model.Item, 0, len(items))
	for _, it := range items {
		if strings.Contains(strings.ToLower(it.Title), q) || strings.Contains(strings.ToLower(it.Symbol), q) {
			out = append(out, it)
		}
	}
	return out
}
