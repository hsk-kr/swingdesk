// Package ui is the lazydocker-style Bubble Tea front end.
package ui

import (
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/hsk-kr/swingdesk/internal/model"
)

type pane int

const (
	paneFilters pane = iota
	paneInbox
	paneDetail
	paneCount
)

// Status is the header line state.
type Status struct {
	LastRefresh time.Time // zero = never
	NextRefresh time.Time // zero = not scheduled
	Agents      string
}

// Options configures a new Model.
type Options struct {
	Instruments []model.Instrument
	Items       []model.Item // unread, newest first
	Biases      map[int64]model.Bias
	Location    *time.Location
	Status      Status
}

// Model is the root Bubble Tea model. Update returns modified copies; slices
// held by the model are never mutated in place.
type Model struct {
	keys        keyMap
	instruments map[int64]model.Instrument
	items       []model.Item
	biases      map[int64]model.Bias
	loc         *time.Location
	status      Status

	filters      []Filter
	filterCursor int
	visible      []model.Item
	itemCursor   int
	detailScroll int
	focus        pane
	showHelp     bool

	width, height int
}

// New builds the root model.
func New(opts Options) Model {
	loc := opts.Location
	if loc == nil {
		loc = time.UTC
	}
	byID := make(map[int64]model.Instrument, len(opts.Instruments))
	for _, in := range opts.Instruments {
		byID[in.ID] = in
	}
	biases := opts.Biases
	if biases == nil {
		biases = map[int64]model.Bias{}
	}
	status := opts.Status
	if status.Agents == "" {
		status.Agents = "idle"
	}
	m := Model{
		keys:        defaultKeys(),
		instruments: byID,
		items:       opts.Items,
		biases:      biases,
		loc:         loc,
		status:      status,
		filters:     buildFilters(opts.Instruments),
		focus:       paneInbox,
	}
	return m.refilter()
}

// refilter recomputes the visible list for the current filter and clamps
// the item cursor.
func (m Model) refilter() Model {
	m.visible = applyFilter(m.items, m.filters[m.filterCursor])
	m.itemCursor = clamp(m.itemCursor, 0, len(m.visible)-1)
	m.detailScroll = 0
	return m
}

// Selected returns the highlighted item, if any.
func (m Model) Selected() (model.Item, bool) {
	if len(m.visible) == 0 {
		return model.Item{}, false
	}
	return m.visible[m.itemCursor], true
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := m.keys
	if m.showHelp {
		if key.Matches(msg, k.Help, k.Back) {
			m.showHelp = false
			return m, nil
		}
		if key.Matches(msg, k.Quit) {
			return m, tea.Quit
		}
		return m, nil
	}
	switch {
	case key.Matches(msg, k.Quit):
		return m, tea.Quit
	case key.Matches(msg, k.Help):
		m.showHelp = true
	case key.Matches(msg, k.NextPane):
		m.focus = (m.focus + 1) % paneCount
	case key.Matches(msg, k.PrevPane):
		m.focus = (m.focus + paneCount - 1) % paneCount
	case key.Matches(msg, k.Left):
		m.focus = max(m.focus-1, paneFilters)
	case key.Matches(msg, k.Right):
		m.focus = min(m.focus+1, paneDetail)
	case key.Matches(msg, k.Enter):
		m.focus = min(m.focus+1, paneDetail)
	case key.Matches(msg, k.Back):
		m.focus = max(m.focus-1, paneFilters)
	case key.Matches(msg, k.Down):
		return m.move(1), nil
	case key.Matches(msg, k.Up):
		return m.move(-1), nil
	case key.Matches(msg, k.Top):
		return m.move(-1 << 30), nil
	case key.Matches(msg, k.Bottom):
		return m.move(1 << 30), nil
	}
	return m, nil
}

// move shifts the cursor of the focused pane by delta, clamped.
func (m Model) move(delta int) Model {
	switch m.focus {
	case paneFilters:
		next := clamp(m.filterCursor+delta, 0, len(m.filters)-1)
		if next != m.filterCursor {
			m.filterCursor = next
			m.itemCursor = 0
			return m.refilter()
		}
	case paneInbox:
		next := clamp(m.itemCursor+delta, 0, len(m.visible)-1)
		if next != m.itemCursor {
			m.itemCursor = next
			m.detailScroll = 0
		}
	case paneDetail:
		m.detailScroll = clamp(m.detailScroll+delta, 0, m.maxDetailScroll())
	}
	return m
}

func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	return min(max(v, lo), hi)
}
