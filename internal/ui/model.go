// Package ui is the lazydocker-style Bubble Tea front end.
package ui

import (
	"fmt"
	"maps"
	"slices"
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
	Agents      model.AgentStatus
}

// Options configures a new Model.
type Options struct {
	Store       Store
	Instruments []model.Instrument
	Location    *time.Location
	Status      Status
	Now         func() time.Time // defaults to time.Now
}

// Model is the root Bubble Tea model. Update returns modified copies; slices
// and maps held by the model are never mutated in place.
type Model struct {
	keys        keyMap
	store       Store
	now         func() time.Time
	instruments map[int64]model.Instrument
	loc         *time.Location
	status      Status

	filters      []Filter
	filterCursor int
	visible      []model.Item
	counts       model.UnreadCounts
	biases       map[int64]model.Bias
	loaded       bool
	loadSeq      int
	undo         [][]int64 // mark-read batches, most recent last

	itemCursor   int
	detailScroll int
	focus        pane
	showHelp     bool
	helpScroll   int
	notice       string
	noticeErr    bool

	width, height int
}

// New builds the root model. The inbox is loaded by Init.
func New(opts Options) Model {
	loc := opts.Location
	if loc == nil {
		loc = time.UTC
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	byID := make(map[int64]model.Instrument, len(opts.Instruments))
	for _, in := range opts.Instruments {
		byID[in.ID] = in
	}
	return Model{
		keys:        defaultKeys(),
		store:       opts.Store,
		now:         now,
		instruments: byID,
		loc:         loc,
		status:      opts.Status,
		filters:     buildFilters(opts.Instruments),
		biases:      map[int64]model.Bias{},
		focus:       paneInbox,
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return loadCmd(m.store, m.loadSeq, m.currentFilter())
}

func (m Model) currentFilter() model.ItemFilter { return m.filters[m.filterCursor].Query }

// reload bumps the sequence (invalidating in-flight loads) and fetches.
func (m Model) reload() (Model, tea.Cmd) {
	m.loadSeq++
	return m, loadCmd(m.store, m.loadSeq, m.currentFilter())
}

// Selected returns the highlighted item, if any.
func (m Model) Selected() (model.Item, bool) {
	if len(m.visible) == 0 {
		return model.Item{}, false
	}
	return m.visible[m.itemCursor], true
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case loadedMsg:
		return m.applyLoaded(msg), nil
	case markedMsg:
		return m.applyMarked(msg)
	case tea.KeyPressMsg:
		m.notice, m.noticeErr = "", false
		return m.handleKey(msg)
	}
	return m, nil
}

// applyLoaded installs a snapshot, keeping the selected item if it is still
// present and otherwise keeping the cursor row (so the next item slides in).
func (m Model) applyLoaded(msg loadedMsg) Model {
	if msg.seq != m.loadSeq || msg.filter != m.currentFilter() {
		return m
	}
	if msg.err != nil {
		return m.withError("load inbox", msg.err)
	}
	prev, hadPrev := m.Selected()
	m.visible = msg.items
	m.counts = msg.counts
	m.biases = msg.biases
	m.loaded = true
	if hadPrev {
		if i := slices.IndexFunc(m.visible, func(it model.Item) bool { return it.ID == prev.ID }); i >= 0 {
			m.itemCursor = i
			return m
		}
		m.detailScroll = 0
	}
	m.itemCursor = clamp(m.itemCursor, 0, len(m.visible)-1)
	return m
}

func (m Model) applyMarked(msg markedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m = m.withError("update read state", msg.err)
		return m.reload()
	}
	switch {
	case msg.undo:
		m.notice = fmt.Sprintf("restored %d", len(msg.ids))
	case len(msg.ids) > 0:
		m.undo = append(slices.Clip(m.undo), msg.ids)
		m.notice = fmt.Sprintf("marked %d read · u undo", len(msg.ids))
	}
	return m.reload()
}

func (m Model) withError(action string, err error) Model {
	m.notice = fmt.Sprintf("%s: %v", action, err)
	m.noticeErr = true
	return m
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.showHelp {
		return m.handleHelpKey(msg)
	}
	k := m.keys
	switch {
	case key.Matches(msg, k.Quit):
		return m, tea.Quit
	case key.Matches(msg, k.Help):
		m.showHelp = true
		m.helpScroll = 0
	case key.Matches(msg, k.MarkRead):
		return m.markSelectedRead()
	case key.Matches(msg, k.MarkAll):
		return m.markVisibleRead()
	case key.Matches(msg, k.Undo):
		return m.undoLast()
	case key.Matches(msg, k.NextPane):
		m.focus = (m.focus + 1) % paneCount
	case key.Matches(msg, k.PrevPane):
		m.focus = (m.focus + paneCount - 1) % paneCount
	case key.Matches(msg, k.Left, k.Back):
		m.focus = max(m.focus-1, paneFilters)
	case key.Matches(msg, k.Right, k.Enter):
		m.focus = min(m.focus+1, paneDetail)
	case key.Matches(msg, k.Down):
		return m.move(1)
	case key.Matches(msg, k.Up):
		return m.move(-1)
	case key.Matches(msg, k.Top):
		return m.move(-1 << 30)
	case key.Matches(msg, k.Bottom):
		return m.move(1 << 30)
	}
	return m, nil
}

// handleHelpKey scrolls or closes the help view; other keys are ignored.
func (m Model) handleHelpKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := m.keys
	maxScroll := m.maxHelpScroll()
	switch {
	case key.Matches(msg, k.Quit):
		return m, tea.Quit
	case key.Matches(msg, k.Help, k.Back):
		m.showHelp = false
	case key.Matches(msg, k.Down):
		m.helpScroll = clamp(m.helpScroll+1, 0, maxScroll)
	case key.Matches(msg, k.Up):
		m.helpScroll = clamp(m.helpScroll-1, 0, maxScroll)
	case key.Matches(msg, k.Top):
		m.helpScroll = 0
	case key.Matches(msg, k.Bottom):
		m.helpScroll = maxScroll
	}
	return m, nil
}

// markSelectedRead optimistically drops the selected row and persists it.
func (m Model) markSelectedRead() (tea.Model, tea.Cmd) {
	it, ok := m.Selected()
	if !ok {
		return m, nil
	}
	m = m.removeLocally([]model.Item{it})
	return m, markReadCmd(m.store, []int64{it.ID}, m.now)
}

// markVisibleRead marks every row under the current filter read.
func (m Model) markVisibleRead() (tea.Model, tea.Cmd) {
	if len(m.visible) == 0 {
		return m, nil
	}
	gone := m.visible
	ids := make([]int64, len(gone))
	for i, it := range gone {
		ids[i] = it.ID
	}
	m = m.removeLocally(gone)
	return m, markReadCmd(m.store, ids, m.now)
}

// undoLast restores the most recent mark-read batch of this session.
func (m Model) undoLast() (tea.Model, tea.Cmd) {
	if len(m.undo) == 0 {
		m.notice = "nothing to undo"
		return m, nil
	}
	last := m.undo[len(m.undo)-1]
	m.undo = m.undo[: len(m.undo)-1 : len(m.undo)-1]
	m.loadSeq++
	return m, markUnreadCmd(m.store, last)
}

// removeLocally hides items and decrements badges before the store confirms,
// so rapid keypresses act on the row the user sees. Any in-flight load is
// invalidated; the post-mark reload reconciles with the DB.
func (m Model) removeLocally(gone []model.Item) Model {
	drop := make(map[int64]bool, len(gone))
	for _, it := range gone {
		drop[it.ID] = true
	}
	m.visible = slices.DeleteFunc(slices.Clone(m.visible), func(it model.Item) bool { return drop[it.ID] })
	m.counts = decrementCounts(m.counts, gone)
	m.itemCursor = clamp(m.itemCursor, 0, len(m.visible)-1)
	m.detailScroll = 0
	m.loadSeq++
	return m
}

func decrementCounts(c model.UnreadCounts, gone []model.Item) model.UnreadCounts {
	out := model.UnreadCounts{
		Total:        c.Total,
		ByCategory:   maps.Clone(c.ByCategory),
		ByInstrument: maps.Clone(c.ByInstrument),
	}
	if out.ByCategory == nil {
		out.ByCategory = map[model.Category]int{}
	}
	if out.ByInstrument == nil {
		out.ByInstrument = map[int64]int{}
	}
	for _, it := range gone {
		out.Total = max(out.Total-1, 0)
		out.ByCategory[it.Category] = max(out.ByCategory[it.Category]-1, 0)
		if it.InstrumentID != 0 {
			out.ByInstrument[it.InstrumentID] = max(out.ByInstrument[it.InstrumentID]-1, 0)
		}
	}
	return out
}

// move shifts the cursor of the focused pane by delta, clamped. Changing the
// left-pane filter triggers a reload.
func (m Model) move(delta int) (Model, tea.Cmd) {
	switch m.focus {
	case paneFilters:
		next := clamp(m.filterCursor+delta, 0, len(m.filters)-1)
		if next == m.filterCursor {
			return m, nil
		}
		m.filterCursor = next
		m.itemCursor = 0
		m.detailScroll = 0
		m.visible = nil
		m.loaded = false
		return m.reload()
	case paneInbox:
		next := clamp(m.itemCursor+delta, 0, len(m.visible)-1)
		if next != m.itemCursor {
			m.itemCursor = next
			m.detailScroll = 0
		}
	case paneDetail:
		m.detailScroll = clamp(m.detailScroll+delta, 0, m.maxDetailScroll())
	}
	return m, nil
}

func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	return min(max(v, lo), hi)
}
