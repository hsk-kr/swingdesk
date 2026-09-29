// Package ui is the lazydocker-style Bubble Tea front end.
package ui

import (
	"fmt"
	"slices"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/hsk-kr/swingdesk/internal/model"
	"github.com/hsk-kr/swingdesk/internal/refresh"
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
	Now         func() time.Time   // defaults to time.Now
	Refresh     RefreshFunc        // nil disables scheduling
	Interval    time.Duration      // refresh interval (default 30m)
	TmuxSession string             // shown in help as `tmux attach -t …`
	Copy        func(string) error // extra clipboard path (tmux buffer); OSC 52 is always sent
	Jobs        []model.Job        // jobs per refresh (default model.DefaultJobs())
	Gate        refresh.Gate       // market-hours gate for scheduled refreshes; nil = always
}

// Model is the root Bubble Tea model. Update returns modified copies; slices
// and maps held by the model are never mutated in place.
type Model struct {
	keys        keyMap
	store       Store
	now         func() time.Time
	refresh     RefreshFunc
	sched       refresh.Schedule
	tick        func() tea.Cmd
	clock       time.Time // last tick; drives the countdown and flash
	flashN      int
	flashUntil  time.Time
	session     string
	jobs        []model.Job
	instruments map[int64]model.Instrument
	loc         *time.Location
	status      Status

	filters      []Filter
	filterCursor int
	items        []model.Item // loaded for the current filter
	visible      []model.Item // items narrowed by query
	query        string       // `/` text filter over title and symbol
	typing       bool         // query input has focus
	showRead     bool         // `s` toggle: include read items
	copyFn       func(string) error
	authProblem  bool // last refresh failed because claude is not logged in
	counts       model.UnreadCounts
	biases       map[int64]model.Bias
	loaded       bool
	loadErr      string
	loadSeq      int
	undo         []undoBatch // mark-read batches ordered by keypress, most recent last
	batchSeq     int         // keypress sequence for mark/undo commands
	inFlight     int         // mark/unmark commands not yet answered

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
	interval := opts.Interval
	if interval <= 0 {
		interval = 30 * time.Minute
	}
	byID := make(map[int64]model.Instrument, len(opts.Instruments))
	for _, in := range opts.Instruments {
		byID[in.ID] = in
	}
	return Model{
		keys:        defaultKeys(),
		store:       opts.Store,
		now:         now,
		refresh:     opts.Refresh,
		sched:       refresh.NewSchedule(interval, opts.Gate),
		jobs:        jobsOrDefault(opts.Jobs),
		tick:        defaultTick,
		clock:       now(),
		session:     opts.TmuxSession,
		copyFn:      opts.Copy,
		instruments: byID,
		loc:         loc,
		status:      opts.Status,
		filters:     buildFilters(opts.Instruments),
		biases:      map[int64]model.Bias{},
		focus:       paneInbox,
	}
}

func jobsOrDefault(jobs []model.Job) []model.Job {
	if len(jobs) == 0 {
		return model.DefaultJobs()
	}
	return slices.Clone(jobs)
}

// undoBatch is one mark-read keypress that can be undone.
type undoBatch struct {
	seq int
	ids []int64
}

// Init implements tea.Model. The first refresh fires on an immediate tick so
// it never blocks the first paint.
func (m Model) Init() tea.Cmd {
	load := loadCmd(m.store, m.loadSeq, m.currentFilter())
	if m.refresh == nil {
		return load
	}
	now := m.now()
	return tea.Batch(load, func() tea.Msg { return tickMsg(now) })
}

func (m Model) currentFilter() model.ItemFilter {
	f := m.filters[m.filterCursor].Query
	f.IncludeRead = m.showRead
	return f
}

// setItems installs the loaded list and re-derives the visible rows.
func (m Model) setItems(items []model.Item) Model {
	m.items = items
	m.visible = matchQuery(items, m.query)
	m.itemCursor = clamp(m.itemCursor, 0, len(m.visible)-1)
	return m
}

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
	case tickMsg:
		return m.onTick(time.Time(msg))
	case jobDoneMsg:
		return m.onJobDone(msg)
	case refreshDoneMsg:
		return m.onRefreshDone(msg.out)
	case copiedMsg:
		return m.onCopied(msg)
	case tea.PasteMsg:
		return m.onPaste(msg)
	case tea.KeyPressMsg:
		if !m.typing {
			m.notice, m.noticeErr = "", false
		}
		return m.handleKey(msg)
	}
	return m, nil
}

// applyLoaded installs a snapshot, keeping the selected item if it is still
// present and otherwise keeping the cursor row (so the next item slides in).
// Snapshots that may predate an in-flight mark are dropped; the reload issued
// when the last mark answers replaces them.
func (m Model) applyLoaded(msg loadedMsg) Model {
	if msg.seq != m.loadSeq || msg.filter != m.currentFilter() || m.inFlight > 0 {
		return m
	}
	m.loaded = true
	if msg.err != nil {
		m.loadErr = msg.err.Error()
		return m.withError("load inbox", msg.err)
	}
	m.loadErr = ""
	prev, hadPrev := m.Selected()
	m = m.setItems(msg.items)
	m.counts = msg.counts
	m.biases = msg.biases
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

// applyMarked records the result of a mark/unmark and reloads once no other
// mark is in flight, so a reload can never observe a half-applied set.
func (m Model) applyMarked(msg markedMsg) (tea.Model, tea.Cmd) {
	m.inFlight = max(m.inFlight-1, 0)
	switch {
	case msg.err != nil && msg.undo:
		m.undo = pushUndo(m.undo, undoBatch{seq: msg.batch, ids: msg.requested})
		m = m.withError("undo", msg.err)
	case msg.err != nil:
		m = m.withError("update read state", msg.err)
	case msg.undo:
		m.notice = fmt.Sprintf("restored %d", len(msg.ids))
	case len(msg.ids) > 0:
		m.undo = pushUndo(m.undo, undoBatch{seq: msg.batch, ids: msg.ids})
		m.notice = fmt.Sprintf("marked %d read · u undo", len(msg.ids))
	}
	if m.inFlight > 0 {
		return m, nil
	}
	return m.reload()
}

// pushUndo returns a new stack with b inserted in keypress order.
func pushUndo(stack []undoBatch, b undoBatch) []undoBatch {
	i := len(stack)
	for i > 0 && stack[i-1].seq > b.seq {
		i--
	}
	return slices.Insert(slices.Clone(stack), i, b)
}

func (m Model) withError(action string, err error) Model {
	m.notice = fmt.Sprintf("%s: %v", action, err)
	m.noticeErr = true
	return m
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case m.showHelp:
		return m.handleHelpKey(msg)
	case m.typing:
		return m.handleQueryKey(msg)
	}
	if next, cmd, ok := m.handleAction(msg); ok {
		return next, cmd
	}
	return m.handleNav(msg)
}

// handleAction handles non-navigation keys; ok is false if msg is not one.
func (m Model) handleAction(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	k := m.keys
	switch {
	case key.Matches(msg, k.Quit):
		return m, tea.Quit, true
	case key.Matches(msg, k.Help):
		m.showHelp, m.helpScroll = true, 0
		return m, nil, true
	case key.Matches(msg, k.Filter):
		m.typing = true
		return m, nil, true
	case key.Matches(msg, k.Back) && m.query != "":
		return m.setQuery(""), nil, true
	case key.Matches(msg, k.MarkRead):
		next, cmd := m.markSelectedRead()
		return next, cmd, true
	case key.Matches(msg, k.MarkAll):
		next, cmd := m.markVisibleRead()
		return next, cmd, true
	case key.Matches(msg, k.Undo):
		next, cmd := m.undoLast()
		return next, cmd, true
	case key.Matches(msg, k.Refresh):
		next, cmd := m.forceRefresh()
		return next, cmd, true
	case key.Matches(msg, k.Copy):
		next, cmd := m.copySelected()
		return next, cmd, true
	case key.Matches(msg, k.ShowRead):
		next, cmd := m.toggleShowRead()
		return next, cmd, true
	}
	return m, nil, false
}

func (m Model) handleNav(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := m.keys
	switch {
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
		m.items, m.visible = nil, nil
		m.loaded = false
		m.loadErr = ""
		if m.inFlight > 0 {
			return m, nil // applyMarked reloads for the new filter
		}
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
