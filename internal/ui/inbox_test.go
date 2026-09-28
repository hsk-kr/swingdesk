package ui

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hsk-kr/swingdesk/internal/model"
)

func visibleIDs(m Model) []int64 {
	out := make([]int64, len(m.visible))
	for i, it := range m.visible {
		out[i] = it.ID
	}
	return out
}

func TestMarkReadRemovesAndSelectsNext(t *testing.T) {
	m, store := newTestModelStore(t, 120, 40)
	m = press(t, m, "j", "j") // select id 3
	m = press(t, m, "r")
	if slices.Contains(visibleIDs(m), 3) {
		t.Fatal("marked item still visible")
	}
	if sel, _ := m.Selected(); sel.ID != 4 {
		t.Errorf("selection = %d, want next row 4", sel.ID)
	}
	if !slices.Equal(store.readIDs(), []int64{3}) {
		t.Errorf("store read = %v", store.readIDs())
	}
	if m.counts.Total != 9 || !strings.Contains(plain(m), "unread 9") {
		t.Errorf("total = %d", m.counts.Total)
	}
	if !strings.Contains(plain(m), "marked 1 read") {
		t.Error("expected notice")
	}
}

func TestMarkReadLastRowSelectsPrevious(t *testing.T) {
	m, _ := newTestModelStore(t, 120, 40)
	m = press(t, m, "G", "r")
	if sel, _ := m.Selected(); sel.ID != 9 {
		t.Errorf("selection = %d, want 9", sel.ID)
	}
}

func TestRapidMarkReadActsOnVisibleRows(t *testing.T) {
	m, store := newTestModelStore(t, 120, 40)
	// Two presses before any command runs: optimistic removal means the
	// second press targets the row that slid into place.
	next, c1 := m.Update(keyMsg("r"))
	next, c2 := next.(Model).Update(keyMsg("r"))
	m = drive(drive(next.(Model), c1), c2)
	if !slices.Equal(store.readIDs(), []int64{1, 2}) {
		t.Errorf("read = %v", store.readIDs())
	}
	if sel, _ := m.Selected(); sel.ID != 3 {
		t.Errorf("selection = %d", sel.ID)
	}
}

func TestMarkAllVisibleUsesFilter(t *testing.T) {
	m, store := newTestModelStore(t, 120, 40)
	m = press(t, m, "h", "j", "j") // Market filter
	m = press(t, m, "a")
	if len(m.visible) != 0 || !strings.Contains(plain(m), "no unread items") {
		t.Errorf("market still has %d", len(m.visible))
	}
	read := store.readIDs()
	if len(read) != 3 {
		t.Fatalf("read = %v", read)
	}
	if m.counts.Total != 7 || m.counts.Count(model.ItemFilter{Category: model.CategoryMarket}) != 0 {
		t.Errorf("counts = %+v", m.counts)
	}
	m = press(t, m, "k", "k") // back to All
	if len(m.visible) != 7 {
		t.Errorf("All shows %d", len(m.visible))
	}
}

func TestUndoRestoresLastBatch(t *testing.T) {
	m, store := newTestModelStore(t, 120, 40)
	m = press(t, m, "r")                // id 1
	m = press(t, m, "h", "j", "j", "a") // market batch
	m = press(t, m, "u")
	if len(store.readIDs()) != 1 || store.readIDs()[0] != 1 {
		t.Errorf("after first undo read = %v", store.readIDs())
	}
	if len(m.visible) != 3 {
		t.Errorf("market restored = %d", len(m.visible))
	}
	m = press(t, m, "u")
	if len(store.readIDs()) != 0 {
		t.Errorf("after second undo read = %v", store.readIDs())
	}
	m = press(t, m, "u")
	if !strings.Contains(plain(m), "nothing to undo") {
		t.Error("expected nothing-to-undo notice")
	}
}

func TestMarkReadNoopOnEmpty(t *testing.T) {
	m := newModelWith(t, newFakeStore(nil, nil), testInstruments(), 100, 30)
	for _, k := range []string{"r", "a"} {
		if _, cmd := m.Update(keyMsg(k)); cmd != nil {
			t.Errorf("%s on empty inbox should not issue a command", k)
		}
	}
}

func TestStoreErrorsShowAndRecover(t *testing.T) {
	m, store := newTestModelStore(t, 120, 40)
	store.s.failNext = errBoom
	m = press(t, m, "r")
	out := plain(m)
	if !strings.Contains(out, "update read state: boom") {
		t.Errorf("expected error notice")
	}
	if len(m.visible) != 10 {
		t.Errorf("failed mark should be reconciled by reload; visible = %d", len(m.visible))
	}
	if len(m.undo) != 0 {
		t.Error("failed mark must not be undoable")
	}
	m = press(t, m, "j")
	if strings.Contains(plain(m), "boom") {
		t.Error("notice should clear on next key")
	}
}

func TestLoadErrorShown(t *testing.T) {
	store := newFakeStore(nil, nil)
	store.s.failNext = errBoom
	m := newModelWith(t, store, testInstruments(), 100, 30)
	if !strings.Contains(plain(m), "load inbox: boom") {
		t.Error("expected load error notice")
	}
}

func TestStaleLoadIgnored(t *testing.T) {
	m, _ := newTestModelStore(t, 120, 40)
	stale := loadedMsg{seq: m.loadSeq - 1, filter: m.currentFilter(), items: nil}
	next, _ := m.Update(stale)
	if len(next.(Model).visible) != 10 {
		t.Error("stale load must be ignored")
	}
	wrongFilter := loadedMsg{seq: m.loadSeq, filter: model.ItemFilter{InstrumentID: 42}}
	next, _ = m.Update(wrongFilter)
	if len(next.(Model).visible) != 10 {
		t.Error("load for another filter must be ignored")
	}
}

func TestReloadKeepsSelectedItem(t *testing.T) {
	m, store := newTestModelStore(t, 120, 40)
	m = press(t, m, "j", "j", "j") // id 4
	// Another item gets read elsewhere (e.g. a later ingest path).
	store.s.items[0].ReadAt = testNow
	m, cmd := m.reload()
	m = drive(m, cmd)
	if sel, _ := m.Selected(); sel.ID != 4 {
		t.Errorf("selection = %d, want 4 kept", sel.ID)
	}
}

func TestDecrementCountsDoesNotMutateInput(t *testing.T) {
	c := model.UnreadCounts{Total: 2, ByCategory: map[model.Category]int{model.CategoryNews: 2}, ByInstrument: map[int64]int{1: 2}}
	out := decrementCounts(c, []model.Item{{InstrumentID: 1, Category: model.CategoryNews}})
	if c.ByCategory[model.CategoryNews] != 2 || c.ByInstrument[1] != 2 || c.Total != 2 {
		t.Error("input mutated")
	}
	if out.Total != 1 || out.ByCategory[model.CategoryNews] != 1 || out.ByInstrument[1] != 1 {
		t.Errorf("out = %+v", out)
	}
}

func run1(t *testing.T, m Model, cmd tea.Cmd) (Model, tea.Cmd) {
	t.Helper()
	next, nc := m.Update(cmd())
	return next.(Model), nc
}

func TestOverlappingMarksNeverResurrectRows(t *testing.T) {
	m, store := newTestModelStore(t, 120, 40)
	next, c1 := m.Update(keyMsg("r"))            // id 1
	next, c2 := next.(Model).Update(keyMsg("r")) // id 2
	m = next.(Model)

	// mark(1) finishes while mark(2) is still in flight: no reload yet.
	m, reload := run1(t, m, c1)
	if reload != nil {
		t.Fatal("must not reload while another mark is in flight")
	}
	// A snapshot taken now (before mark(2) commits) still contains id 2.
	stale, _ := store.Unread(t.Context(), m.currentFilter())
	counts, _ := store.Counts(t.Context())
	next, _ = m.Update(loadedMsg{seq: m.loadSeq, filter: m.currentFilter(), items: stale, counts: counts})
	m = next.(Model)
	if slices.Contains(visibleIDs(m), 2) || m.counts.Total != 8 {
		t.Fatalf("ghost row: visible=%v total=%d", visibleIDs(m), m.counts.Total)
	}

	m = drive(m, c2)
	if slices.Contains(visibleIDs(m), 1) || slices.Contains(visibleIDs(m), 2) || m.counts.Total != 8 {
		t.Errorf("after both marks: visible=%v total=%d", visibleIDs(m), m.counts.Total)
	}
}

func TestUndoFollowsKeypressOrderWhenMarksFinishOutOfOrder(t *testing.T) {
	m, store := newTestModelStore(t, 120, 40)
	next, c1 := m.Update(keyMsg("r"))            // id 1, batch 1
	next, c2 := next.(Model).Update(keyMsg("r")) // id 2, batch 2
	m = drive(drive(next.(Model), c2), c1)       // finish 2 before 1
	m = press(t, m, "u")
	if !slices.Equal(store.readIDs(), []int64{1}) {
		t.Errorf("undo should restore the last key press (id 2); read = %v", store.readIDs())
	}
}

func TestFailedUndoKeepsBatch(t *testing.T) {
	m, store := newTestModelStore(t, 120, 40)
	m = press(t, m, "r")
	store.s.failNext = errBoom
	m = press(t, m, "u")
	if !strings.Contains(plain(m), "undo: boom") {
		t.Error("expected undo error notice")
	}
	m = press(t, m, "u")
	if len(store.readIDs()) != 0 {
		t.Errorf("retrying undo should restore; read = %v", store.readIDs())
	}
}

func TestLoadErrorAfterFilterChangeIsNotStuckLoading(t *testing.T) {
	m, store := newTestModelStore(t, 120, 40)
	m = press(t, m, "h")
	store.s.failNext = errBoom
	m = press(t, m, "j", "l")
	out := plain(m)
	if strings.Contains(out, "loading…") || !strings.Contains(out, "failed to load: boom") {
		t.Errorf("inbox should show the load error, got:\n%s", out)
	}
	m = press(t, m, "h", "k")
	if len(m.visible) != 10 || strings.Contains(plain(m), "failed to load") {
		t.Error("changing filter should retry")
	}
}

func TestFilterChangeDuringMarkReloadsAfterMark(t *testing.T) {
	m, _ := newTestModelStore(t, 120, 40)
	next, c1 := m.Update(keyMsg("r"))
	m = press(t, next.(Model), "h", "j", "j") // Market while mark in flight
	if m.loaded {
		t.Fatal("filter change during a mark should wait")
	}
	m = drive(m, c1)
	if !m.loaded || len(m.visible) != 3 {
		t.Errorf("market after mark: loaded=%v visible=%d", m.loaded, len(m.visible))
	}
}
