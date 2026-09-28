package db

import (
	"context"
	"database/sql"
	"slices"
	"testing"
	"time"

	"github.com/hsk-kr/swingdesk/internal/model"
)

var t0 = time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)

func seededDB(t *testing.T) (*sql.DB, map[string]int64) {
	t.Helper()
	conn, _ := openTemp(t)
	ctx := context.Background()
	if _, err := SeedInstruments(ctx, conn, seed(t)); err != nil {
		t.Fatal(err)
	}
	ins, err := ListInstruments(ctx, conn)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]int64{}
	for _, in := range ins {
		ids[in.Symbol] = in.ID
	}
	return conn, ids
}

func insert(t *testing.T, conn *sql.DB, it model.Item) int64 {
	t.Helper()
	id, ok, err := InsertItem(context.Background(), conn, it)
	if err != nil || !ok {
		t.Fatalf("InsertItem(%q) = %v, %v", it.Title, ok, err)
	}
	return id
}

func titles(items []model.Item) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Title
	}
	return out
}

func TestUnreadItemsFiltersAndOrder(t *testing.T) {
	conn, ids := seededDB(t)
	ctx := context.Background()
	insert(t, conn, model.Item{InstrumentID: ids["NVDA"], Symbol: "NVDA", Category: model.CategoryNews, Title: "old nvda", CreatedAt: t0})
	insert(t, conn, model.Item{InstrumentID: ids["NVDA"], Symbol: "NVDA", Category: model.CategoryOpinion, Title: "new nvda", CreatedAt: t0.Add(2 * time.Hour), URL: "https://x", Source: "Reuters", PublishedAt: t0})
	insert(t, conn, model.Item{Category: model.CategoryMarket, Title: "tape", CreatedAt: t0.Add(time.Hour)})
	// Sub-second precision must still sort correctly.
	insert(t, conn, model.Item{Category: model.CategoryTech, Title: "tech frac", CreatedAt: t0.Add(time.Hour + 500*time.Millisecond)})

	all, err := UnreadItems(ctx, conn, model.ItemFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := titles(all), []string{"new nvda", "tech frac", "tape", "old nvda"}; !slices.Equal(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
	first := all[0]
	if first.Symbol != "NVDA" || first.Source != "Reuters" || first.URL != "https://x" || !first.PublishedAt.Equal(t0) || !first.ReadAt.IsZero() {
		t.Errorf("round trip = %+v", first)
	}
	if all[2].Symbol != "" || all[2].InstrumentID != 0 {
		t.Errorf("general item should have no instrument: %+v", all[2])
	}

	nv, _ := UnreadItems(ctx, conn, model.ItemFilter{InstrumentID: ids["NVDA"]})
	if got := titles(nv); !slices.Equal(got, []string{"new nvda", "old nvda"}) {
		t.Errorf("NVDA filter = %v", got)
	}
	mk, _ := UnreadItems(ctx, conn, model.ItemFilter{Category: model.CategoryMarket})
	if got := titles(mk); !slices.Equal(got, []string{"tape"}) {
		t.Errorf("market filter = %v", got)
	}
	both, _ := UnreadItems(ctx, conn, model.ItemFilter{InstrumentID: ids["NVDA"], Category: model.CategoryNews})
	if got := titles(both); !slices.Equal(got, []string{"old nvda"}) {
		t.Errorf("combined filter = %v", got)
	}
}

func TestMarkReadUnreadAndCounts(t *testing.T) {
	conn, ids := seededDB(t)
	ctx := context.Background()
	a := insert(t, conn, model.Item{InstrumentID: ids["NVDA"], Symbol: "NVDA", Category: model.CategoryNews, Title: "a", CreatedAt: t0})
	b := insert(t, conn, model.Item{InstrumentID: ids["NVDA"], Symbol: "NVDA", Category: model.CategoryNews, Title: "b", CreatedAt: t0})
	c := insert(t, conn, model.Item{Category: model.CategoryTech, Title: "c", CreatedAt: t0})

	counts, err := CountUnread(ctx, conn)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Total != 3 || counts.ByInstrument[ids["NVDA"]] != 2 || counts.ByCategory[model.CategoryTech] != 1 || counts.ByCategory[model.CategoryNews] != 2 {
		t.Errorf("counts = %+v", counts)
	}
	if counts.Count(model.ItemFilter{}) != 3 || counts.Count(model.ItemFilter{InstrumentID: ids["NVDA"]}) != 2 {
		t.Error("Count helper wrong")
	}

	changed, err := MarkRead(ctx, conn, []int64{a, c, 9999}, t0.Add(time.Hour))
	if err != nil || !slices.Equal(changed, []int64{a, c}) {
		t.Fatalf("MarkRead = %v, %v", changed, err)
	}
	again, err := MarkRead(ctx, conn, []int64{a}, t0.Add(2*time.Hour))
	if err != nil || len(again) != 0 {
		t.Errorf("re-marking should be a no-op: %v %v", again, err)
	}
	var readAt string
	if err := conn.QueryRow(`SELECT read_at FROM items WHERE id = ?`, a).Scan(&readAt); err != nil || readAt != formatTime(t0.Add(time.Hour)) {
		t.Errorf("read_at = %q (%v); first mark time must be kept", readAt, err)
	}
	unread, _ := UnreadItems(ctx, conn, model.ItemFilter{})
	if got := titles(unread); !slices.Equal(got, []string{"b"}) {
		t.Errorf("unread after mark = %v", got)
	}

	restored, err := MarkUnread(ctx, conn, []int64{a, c, b})
	if err != nil || !slices.Equal(restored, []int64{a, c}) {
		t.Fatalf("MarkUnread = %v, %v", restored, err)
	}
	counts, _ = CountUnread(ctx, conn)
	if counts.Total != 3 {
		t.Errorf("total after undo = %d", counts.Total)
	}
	if got, err := MarkRead(ctx, conn, nil, t0); got != nil || err != nil {
		t.Error("empty ids should be a no-op")
	}
}

func TestInsertItemDedupes(t *testing.T) {
	conn, _ := seededDB(t)
	it := model.Item{Symbol: "NVDA", Category: model.CategoryNews, Title: "Same", URL: "u", CreatedAt: t0}
	insert(t, conn, it)
	it.Title = "SAME"
	if _, ok, err := InsertItem(context.Background(), conn, it); err != nil || ok {
		t.Errorf("duplicate insert = %v, %v", ok, err)
	}
	it.Category = "bogus"
	if _, _, err := InsertItem(context.Background(), conn, it); err == nil {
		t.Error("invalid category should fail")
	}
}

func TestDedupeKey(t *testing.T) {
	if DedupeKey("NVDA", "Title", "u") != DedupeKey("nvda", "TITLE", "u") {
		t.Error("symbol and title must be case-insensitive")
	}
	if DedupeKey("NVDA", "Title", "u") == DedupeKey("NVDA", "Title", "U") {
		t.Error("url is case-sensitive")
	}
	// sha256("nvda|title|u")
	if got := DedupeKey("NVDA", "Title", "u"); len(got) != 64 {
		t.Errorf("key = %s", got)
	}
}

func TestLatestBiases(t *testing.T) {
	conn, ids := seededDB(t)
	ctx := context.Background()
	nv := ids["NVDA"]
	for i, s := range []model.Stance{model.StanceLong, model.StanceShort} {
		if err := InsertBias(ctx, conn, 0, model.Bias{InstrumentID: nv, Stance: s, Confidence: 0.5, Rationale: string(s), CreatedAt: t0.Add(time.Duration(i) * time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := InsertBias(ctx, conn, 0, model.Bias{InstrumentID: nv, Stance: "flat"}); err == nil {
		t.Error("invalid stance should fail")
	}
	got, err := LatestBiases(ctx, conn)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[nv].Stance != model.StanceShort || !got[nv].CreatedAt.Equal(t0.Add(time.Hour)) {
		t.Errorf("latest = %+v", got)
	}
}
