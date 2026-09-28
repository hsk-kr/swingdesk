package ui

import (
	"context"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/hsk-kr/swingdesk/internal/model"
)

// storeTimeout bounds each store call so a locked DB cannot hang the UI.
const storeTimeout = 5 * time.Second

// Store is the inbox persistence the UI depends on.
type Store interface {
	Unread(ctx context.Context, f model.ItemFilter) ([]model.Item, error)
	Counts(ctx context.Context) (model.UnreadCounts, error)
	Biases(ctx context.Context) (map[int64]model.Bias, error)
	MarkRead(ctx context.Context, ids []int64, at time.Time) ([]int64, error)
	MarkUnread(ctx context.Context, ids []int64) ([]int64, error)
}

// loadedMsg carries a fresh inbox snapshot. seq lets the model drop
// responses that were overtaken by a newer load or a local mutation.
type loadedMsg struct {
	seq    int
	filter model.ItemFilter
	items  []model.Item
	counts model.UnreadCounts
	biases map[int64]model.Bias
	err    error
}

// markedMsg reports a mark-read (undo=false) or mark-unread (undo=true).
// batch is the keypress sequence number; requested is what was asked for,
// ids what actually changed.
type markedMsg struct {
	batch     int
	requested []int64
	ids       []int64
	undo      bool
	err       error
}

func loadCmd(s Store, seq int, f model.ItemFilter) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), storeTimeout)
		defer cancel()
		msg := loadedMsg{seq: seq, filter: f}
		if msg.items, msg.err = s.Unread(ctx, f); msg.err != nil {
			return msg
		}
		if msg.counts, msg.err = s.Counts(ctx); msg.err != nil {
			return msg
		}
		msg.biases, msg.err = s.Biases(ctx)
		return msg
	}
}

func markReadCmd(s Store, batch int, ids []int64, now func() time.Time) tea.Cmd {
	ids = slices.Clone(ids)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), storeTimeout)
		defer cancel()
		changed, err := s.MarkRead(ctx, ids, now())
		return markedMsg{batch: batch, requested: ids, ids: changed, err: err}
	}
}

func markUnreadCmd(s Store, batch int, ids []int64) tea.Cmd {
	ids = slices.Clone(ids)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), storeTimeout)
		defer cancel()
		changed, err := s.MarkUnread(ctx, ids)
		return markedMsg{batch: batch, requested: ids, ids: changed, undo: true, err: err}
	}
}
