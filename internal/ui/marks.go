package ui

import (
	"maps"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/hsk-kr/swingdesk/internal/model"
)

// markSelectedRead optimistically updates the selected row and persists it.
func (m Model) markSelectedRead() (tea.Model, tea.Cmd) {
	it, ok := m.Selected()
	if !ok {
		return m, nil
	}
	if !it.ReadAt.IsZero() {
		m.notice = "already read · u undoes the last mark"
		return m, nil
	}
	m = m.applyReadLocally([]model.Item{it})
	return m.startMark(func(batch int) tea.Cmd { return markReadCmd(m.store, batch, []int64{it.ID}, m.now) })
}

// markVisibleRead marks every unread row currently shown (filter + query).
func (m Model) markVisibleRead() (tea.Model, tea.Cmd) {
	gone := slices.DeleteFunc(slices.Clone(m.visible), func(it model.Item) bool { return !it.ReadAt.IsZero() })
	if len(gone) == 0 {
		return m, nil
	}
	ids := make([]int64, len(gone))
	for i, it := range gone {
		ids[i] = it.ID
	}
	m = m.applyReadLocally(gone)
	return m.startMark(func(batch int) tea.Cmd { return markReadCmd(m.store, batch, ids, m.now) })
}

// startMark allocates a keypress sequence number and counts the command as
// in flight.
func (m Model) startMark(cmd func(batch int) tea.Cmd) (Model, tea.Cmd) {
	m.batchSeq++
	m.inFlight++
	m.loadSeq++
	return m, cmd(m.batchSeq)
}

// undoLast restores the most recent mark-read batch of this session.
func (m Model) undoLast() (tea.Model, tea.Cmd) {
	if len(m.undo) == 0 {
		m.notice = "nothing to undo"
		return m, nil
	}
	last := m.undo[len(m.undo)-1]
	m.undo = m.undo[: len(m.undo)-1 : len(m.undo)-1]
	return m.startMark(func(int) tea.Cmd { return markUnreadCmd(m.store, last.seq, last.ids) })
}

// applyReadLocally reflects a mark-read before the store confirms, so rapid
// keypresses act on the row the user sees: rows disappear, or are shown as
// read when the show-read toggle is on. The reload after the last in-flight
// mark reconciles with the DB.
func (m Model) applyReadLocally(gone []model.Item) Model {
	ids := make(map[int64]bool, len(gone))
	for _, it := range gone {
		ids[it.ID] = true
	}
	var items []model.Item
	if m.showRead {
		items = markedRead(m.items, ids, m.now())
	} else {
		items = slices.DeleteFunc(slices.Clone(m.items), func(it model.Item) bool { return ids[it.ID] })
	}
	m = m.setItems(items)
	m.counts = decrementCounts(m.counts, gone)
	m.detailScroll = 0
	return m
}

func markedRead(items []model.Item, ids map[int64]bool, at time.Time) []model.Item {
	out := slices.Clone(items)
	for i, it := range out {
		if ids[it.ID] {
			out[i].ReadAt = at
		}
	}
	return out
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

// toggleShowRead flips the `s` toggle and reloads.
func (m Model) toggleShowRead() (tea.Model, tea.Cmd) {
	m.showRead = !m.showRead
	if m.showRead {
		m.notice = "showing read items · s to hide"
	} else {
		m.notice = "unread only"
	}
	if m.inFlight > 0 {
		return m, nil // applyMarked reloads with the new filter
	}
	return m.reload()
}
