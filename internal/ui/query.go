package ui

import (
	"slices"
	"strings"
	"unicode"

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

// setQuery re-derives the visible rows for q, keeping the selected item
// when it is still shown (else the top row).
func (m Model) setQuery(q string) Model {
	prev, hadPrev := m.Selected()
	m.query = q
	m.itemCursor = 0
	m.detailScroll = 0
	m = m.setItems(m.items)
	if hadPrev {
		if i := slices.IndexFunc(m.visible, func(it model.Item) bool { return it.ID == prev.ID }); i >= 0 {
			m.itemCursor = i
		}
	}
	return m
}

// onPaste appends pasted text (bracketed paste) to the query while typing.
func (m Model) onPaste(msg tea.PasteMsg) (tea.Model, tea.Cmd) {
	if !m.typing {
		return m, nil
	}
	clean := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, msg.Content)
	return m.setQuery(m.query + clean), nil
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
