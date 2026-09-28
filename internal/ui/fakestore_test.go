package ui

import (
	"context"
	"errors"
	"slices"
	"sort"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/hsk-kr/swingdesk/internal/model"
)

// fakeStore is an in-memory Store. Methods use value receivers over a
// pointer to shared state so the model can hold it by value.
type fakeStore struct{ s *fakeState }

type fakeState struct {
	items    []model.Item
	biases   map[int64]model.Bias
	failNext error
}

func newFakeStore(items []model.Item, biases map[int64]model.Bias) fakeStore {
	return fakeStore{s: &fakeState{items: slices.Clone(items), biases: biases}}
}

func (f fakeStore) takeErr() error {
	err := f.s.failNext
	f.s.failNext = nil
	return err
}

func (f fakeStore) Unread(_ context.Context, q model.ItemFilter) ([]model.Item, error) {
	if err := f.takeErr(); err != nil {
		return nil, err
	}
	var out []model.Item
	for _, it := range f.s.items {
		if !it.ReadAt.IsZero() {
			continue
		}
		if q.InstrumentID != 0 && it.InstrumentID != q.InstrumentID {
			continue
		}
		if q.Category != "" && it.Category != q.Category {
			continue
		}
		out = append(out, it)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (f fakeStore) Counts(context.Context) (model.UnreadCounts, error) {
	c := model.UnreadCounts{ByCategory: map[model.Category]int{}, ByInstrument: map[int64]int{}}
	for _, it := range f.s.items {
		if !it.ReadAt.IsZero() {
			continue
		}
		c.Total++
		c.ByCategory[it.Category]++
		if it.InstrumentID != 0 {
			c.ByInstrument[it.InstrumentID]++
		}
	}
	return c, nil
}

func (f fakeStore) Biases(context.Context) (map[int64]model.Bias, error) {
	if f.s.biases == nil {
		return map[int64]model.Bias{}, nil
	}
	return f.s.biases, nil
}

func (f fakeStore) setRead(ids []int64, at time.Time, wantUnread bool) []int64 {
	var changed []int64
	for i, it := range f.s.items {
		if slices.Contains(ids, it.ID) && it.ReadAt.IsZero() == wantUnread {
			f.s.items[i].ReadAt = at
			changed = append(changed, it.ID)
		}
	}
	return changed
}

func (f fakeStore) MarkRead(_ context.Context, ids []int64, at time.Time) ([]int64, error) {
	if err := f.takeErr(); err != nil {
		return nil, err
	}
	return f.setRead(ids, at, true), nil
}

func (f fakeStore) MarkUnread(_ context.Context, ids []int64) ([]int64, error) {
	if err := f.takeErr(); err != nil {
		return nil, err
	}
	return f.setRead(ids, time.Time{}, false), nil
}

func (f fakeStore) readIDs() []int64 {
	var out []int64
	for _, it := range f.s.items {
		if !it.ReadAt.IsZero() {
			out = append(out, it.ID)
		}
	}
	return out
}

var errBoom = errors.New("boom")

// drive runs cmd synchronously, feeding every resulting message back into m
// until no commands remain.
func drive(m Model, cmd tea.Cmd) Model {
	queue := []tea.Cmd{cmd}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		msg := c()
		if batch, ok := msg.(tea.BatchMsg); ok {
			queue = append(queue, batch...)
			continue
		}
		if _, ok := msg.(tea.QuitMsg); ok {
			continue
		}
		next, nc := m.Update(msg)
		m = next.(Model)
		queue = append(queue, nc)
	}
	return m
}
