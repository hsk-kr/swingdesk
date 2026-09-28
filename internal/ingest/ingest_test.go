package ingest

import (
	"bytes"
	"context"
	"database/sql"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hsk-kr/swingdesk"
	"github.com/hsk-kr/swingdesk/internal/db"
	"github.com/hsk-kr/swingdesk/internal/model"
	"github.com/hsk-kr/swingdesk/internal/watchlist"
)

var now = time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()
	conn, err := db.Open(ctx, filepath.Join(t.TempDir(), "i.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	seed, err := watchlist.Parse(swingdesk.WatchlistYAML)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SeedInstruments(ctx, conn, seed); err != nil {
		t.Fatal(err)
	}
	return conn
}

func ingestFixture(t *testing.T, conn *sql.DB, name string, opts Options) Result {
	t.Helper()
	if opts.Now.IsZero() {
		opts.Now = now
	}
	res, err := IngestFile(context.Background(), conn, filepath.Join("testdata", name), opts)
	if err != nil {
		t.Fatalf("IngestFile(%s): %v", name, err)
	}
	return res
}

func count(t *testing.T, conn *sql.DB, q string, args ...any) int {
	t.Helper()
	var n int
	if err := conn.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return n
}

func TestIngestMarket(t *testing.T) {
	conn := openDB(t)
	res := ingestFixture(t, conn, "market.json", Options{})
	if res.Job != model.JobMarket || res.Inserted != 3 || res.Events != 1 || res.Biases != 2 || len(res.Skipped) != 0 {
		t.Fatalf("result = %+v", res)
	}
	if n := count(t, conn, `SELECT COUNT(*) FROM items WHERE instrument_id IS NULL`); n != 1 {
		t.Errorf("general items = %d", n)
	}
	var kind, at string
	if err := conn.QueryRow(`SELECT e.kind, e.event_at FROM events e JOIN instruments i ON i.id = e.instrument_id WHERE i.symbol = 'CRUDE'`).Scan(&kind, &at); err != nil {
		t.Fatal(err)
	}
	if kind != "macro" || !strings.HasPrefix(at, "2026-10-05T00:00:00") {
		t.Errorf("event = %s %s", kind, at)
	}
	var url sql.NullString
	if err := conn.QueryRow(`SELECT url FROM items WHERE title = 'OPEC+ meeting'`).Scan(&url); err != nil || url.Valid {
		t.Errorf("url-less event item should store NULL url: %v %v", url, err)
	}
}

func TestIngestNamesSkipsBadEntriesButKeepsFile(t *testing.T) {
	conn := openDB(t)
	res := ingestFixture(t, conn, "names.json", Options{RunID: 0})
	if res.Inserted != 2 || res.Events != 1 || res.Biases != 2 {
		t.Errorf("result = %+v", res)
	}
	reasons := map[string]string{}
	for _, s := range res.Skipped {
		reasons[string(s.Kind)+":"+s.Symbol] = s.Reason
	}
	for key, want := range map[string]string{
		"item:IBM":  "unknown symbol",
		"item:META": "invalid category",
		"item:":     "need a symbol",
		"bias:IBM":  "unknown symbol",
		"bias:META": "confidence",
	} {
		if !strings.Contains(reasons[key], want) {
			t.Errorf("skip %s = %q, want %q", key, reasons[key], want)
		}
	}
	if n := count(t, conn, `SELECT COUNT(*) FROM items`); n != 2 {
		t.Errorf("items = %d", n)
	}
}

func TestIngestTechAndClaudeWrapper(t *testing.T) {
	conn := openDB(t)
	if res := ingestFixture(t, conn, "tech.json", Options{}); res.Inserted != 2 {
		t.Errorf("tech = %+v", res)
	}
	res := ingestFixture(t, conn, "claude_wrapped.json", Options{})
	if res.Job != model.JobTech || res.Inserted != 1 {
		t.Errorf("wrapped = %+v", res)
	}
}

func TestInvalidFileRejected(t *testing.T) {
	conn := openDB(t)
	_, err := IngestFile(context.Background(), conn, filepath.Join("testdata", "invalid.json"), Options{Now: now})
	if err == nil || !strings.Contains(err.Error(), "job") || !strings.Contains(err.Error(), "generated_at") {
		t.Fatalf("err = %v", err)
	}
	if n := count(t, conn, `SELECT COUNT(*) FROM items`); n != 0 {
		t.Errorf("items = %d", n)
	}
}

func TestReingestUpdatesAndKeepsReadAt(t *testing.T) {
	conn := openDB(t)
	ctx := context.Background()
	ingestFixture(t, conn, "names.json", Options{})
	var id int64
	if err := conn.QueryRow(`SELECT id FROM items WHERE url = 'https://example.com/nvda-capex'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.MarkRead(ctx, conn, []int64{id}, now); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join("testdata", "names.json"))
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(raw), "Two cloud providers raised capex guidance.", "Three providers now.", 1)
	edited = strings.Replace(edited, `"source": "Reuters"`, `"source": "Bloomberg"`, 1)
	// Title case differs; dedupe key lowercases the title.
	edited = strings.Replace(edited, "Hyperscaler capex guide lifts accelerator orders", "HYPERSCALER CAPEX GUIDE LIFTS ACCELERATOR ORDERS", 1)
	env, err := Parse([]byte(edited))
	if err != nil {
		t.Fatal(err)
	}
	res, err := Ingest(ctx, conn, env, Options{Now: now.Add(30 * time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if res.Inserted != 0 || res.Updated != 2 {
		t.Errorf("re-ingest = %+v", res)
	}
	var summary, source, readAt, created string
	if err := conn.QueryRow(`SELECT summary, source, COALESCE(read_at, ''), created_at FROM items WHERE id = ?`, id).
		Scan(&summary, &source, &readAt, &created); err != nil {
		t.Fatal(err)
	}
	if summary != "Three providers now." || source != "Bloomberg" || readAt == "" || !strings.HasPrefix(created, "2026-09-27T12:00:00") {
		t.Errorf("row = %q %q read=%q created=%q", summary, source, readAt, created)
	}
	if n := count(t, conn, `SELECT COUNT(*) FROM events`); n != 1 {
		t.Errorf("events after re-ingest = %d (must not duplicate)", n)
	}
	if n := count(t, conn, `SELECT COUNT(*) FROM biases`); n != 4 {
		t.Errorf("biases = %d (history kept per run)", n)
	}
}

func TestCapAndLogging(t *testing.T) {
	conn := openDB(t)
	var logs bytes.Buffer
	res := ingestFixture(t, conn, "market.json", Options{MaxItems: 1, Logger: slog.New(slog.NewTextHandler(&logs, nil))})
	if res.Inserted != 1 || len(res.Skipped) != 2 || !strings.Contains(res.Skipped[0].Reason, "over cap") {
		t.Errorf("result = %+v", res)
	}
	if !strings.Contains(logs.String(), "ingest skipped") {
		t.Errorf("expected skip log, got %q", logs.String())
	}
}

func TestRunIDRecorded(t *testing.T) {
	conn := openDB(t)
	r, err := conn.Exec(`INSERT INTO refresh_runs (started_at, status) VALUES ('2026-09-27T12:00:00.000000000Z', 'running')`)
	if err != nil {
		t.Fatal(err)
	}
	runID, _ := r.LastInsertId()
	ingestFixture(t, conn, "tech.json", Options{RunID: runID})
	if n := count(t, conn, `SELECT COUNT(*) FROM items WHERE run_id = ?`, runID); n != 2 {
		t.Errorf("items with run_id = %d", n)
	}
}

func TestParseUnwrapVariants(t *testing.T) {
	env := `{"job":"tech","generated_at":"2026-09-27T12:00:00Z","items":[],"biases":[]}`
	cases := map[string]struct {
		raw     string
		wantErr string
	}{
		"bare":            {raw: env},
		"result string":   {raw: `{"type":"result","is_error":false,"result":` + quote(env) + `}`},
		"fenced result":   {raw: `{"type":"result","is_error":false,"result":` + quote("```json\n"+env+"\n```") + `}`},
		"claude error":    {raw: `{"type":"result","is_error":true,"result":"Not logged in"}`, wantErr: "Not logged in"},
		"no payload":      {raw: `{"type":"result","is_error":false}`, wantErr: "neither"},
		"not json":        {raw: `nope`, wantErr: "decode"},
		"missing gen at":  {raw: `{"job":"tech","items":[],"biases":[]}`, wantErr: "generated_at"},
		"missing lists":   {raw: `{"job":"tech","generated_at":"2026-09-27T12:00:00Z"}`, wantErr: `missing "items"`},
		"null biases":     {raw: `{"job":"tech","generated_at":"2026-09-27T12:00:00Z","items":[],"biases":null}`, wantErr: `missing "biases"`},
		"prose + fence":   {raw: `{"type":"result","is_error":false,"result":` + quote("Here is the envelope:\n```json\n"+env+"\n```") + `}`},
		"trailing prose":  {raw: `{"type":"result","is_error":false,"result":` + quote(env+"\n\nLet me know if you need more.") + `}`},
		"one-line fence":  {raw: `{"type":"result","is_error":false,"result":` + quote("```json "+env+"```") + `}`},
		"brace in prose":  {raw: `{"type":"result","is_error":false,"result":` + quote("Note {not json} then "+env) + `}`},
		"no object":       {raw: `{"type":"result","is_error":false,"result":"I could not find news."}`, wantErr: "no JSON object"},
		"wrong item type": {raw: `{"job":"tech","generated_at":"2026-09-27T12:00:00Z","items":{}}`, wantErr: "decode envelope"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(c.raw))
			if c.wantErr == "" && err != nil {
				t.Fatalf("unexpected err %v", err)
			}
			if c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)) {
				t.Fatalf("err = %v, want %q", err, c.wantErr)
			}
		})
	}
}

func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func TestCheckItemRules(t *testing.T) {
	known := map[string]model.Instrument{"NVDA": {ID: 1, Symbol: "NVDA"}}
	s := func(v string) *string { return &v }
	cases := map[string]struct {
		job  model.Job
		it   RawItem
		fail bool
	}{
		"ok":             {model.JobNames, RawItem{Symbol: s("nvda"), Category: "news", Title: "t", URL: "https://x.y"}, false},
		"general tech":   {model.JobTech, RawItem{Category: "tech", Title: "t", URL: "https://x.y"}, false},
		"blank title":    {model.JobTech, RawItem{Category: "tech", Title: "  ", URL: "https://x.y"}, true},
		"ftp url":        {model.JobTech, RawItem{Category: "tech", Title: "t", URL: "ftp://x.y"}, true},
		"no url no ev":   {model.JobTech, RawItem{Category: "tech", Title: "t"}, true},
		"no url bad ev":  {model.JobTech, RawItem{Category: "tech", Title: "t", EventAt: s("soon")}, true},
		"no url date ev": {model.JobTech, RawItem{Category: "event", Title: "t", EventAt: s("2026-10-01")}, false},
	}
	for name, c := range cases {
		_, err := checkItem(c.job, c.it, known)
		if (err != nil) != c.fail {
			t.Errorf("%s: err = %v, fail = %v", name, err, c.fail)
		}
	}
}

func TestEventKindFallback(t *testing.T) {
	s := func(v string) *string { return &v }
	if eventKind(nil) != model.EventOther || eventKind(s("IPO")) != model.EventOther || eventKind(s("Earnings")) != model.EventEarnings {
		t.Error("event kind mapping wrong")
	}
}

func TestBadTimestampsAreReportedNotSilent(t *testing.T) {
	conn := openDB(t)
	raw := `{"job":"names","generated_at":"2026-09-27T12:00:00Z","items":[
	  {"symbol":"NVDA","category":"event","title":"Earnings","summary":"s","source":"IR","url":"https://example.com/e",
	   "published_at":"last week","event_at":"2026-10-28T20:00","event_kind":"earnings"}],"biases":[]}`
	env, err := Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	res, err := Ingest(context.Background(), conn, env, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if res.Inserted != 1 || res.Events != 0 || len(res.Warnings) != 2 {
		t.Fatalf("result = %+v", res)
	}
	kinds := []SkipKind{res.Warnings[0].Kind, res.Warnings[1].Kind}
	if kinds[0] != SkipPublishedAt || kinds[1] != SkipEvent || res.Warnings[1].Symbol != "NVDA" {
		t.Errorf("warnings = %+v", res.Warnings)
	}
}

func TestUpdateWithEmptyFieldsKeepsData(t *testing.T) {
	conn := openDB(t)
	raw := `{"job":"names","generated_at":"2026-09-27T12:00:00Z","items":[
	  {"symbol":"NVDA","category":"news","title":"Dup","summary":"s","body":"b","source":"R","url":"https://example.com/d"},
	  {"symbol":"NVDA","category":"news","title":"  dup ","summary":"s2","body":"","source":"","url":"https://example.com/d"}],"biases":[]}`
	env, err := Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	res, err := Ingest(context.Background(), conn, env, Options{Now: now})
	if err != nil || res.Inserted != 1 || res.Updated != 1 {
		t.Fatalf("res = %+v, %v", res, err)
	}
	var summary, body, source string
	if err := conn.QueryRow(`SELECT summary, body, source FROM items`).Scan(&summary, &body, &source); err != nil {
		t.Fatal(err)
	}
	if summary != "s2" || body != "b" || source != "R" {
		t.Errorf("row = %q %q %q", summary, body, source)
	}
}

func TestDuplicateBiasInFileSkipped(t *testing.T) {
	conn := openDB(t)
	raw := `{"job":"names","generated_at":"2026-09-27T12:00:00Z","items":[],"biases":[
	  {"symbol":"NVDA","stance":"long","confidence":0.4,"rationale":"a"},
	  {"symbol":"nvda","stance":"short","confidence":0.5,"rationale":"b"}]}`
	env, _ := Parse([]byte(raw))
	res, err := Ingest(context.Background(), conn, env, Options{Now: now})
	if err != nil || res.Biases != 1 || len(res.Skipped) != 1 || !strings.Contains(res.Skipped[0].Reason, "duplicate") {
		t.Errorf("res = %+v, %v", res, err)
	}
}

func TestWhitespaceURLTreatedAsMissing(t *testing.T) {
	s := func(v string) *string { return &v }
	it := RawItem{Category: "event", Title: "t", URL: "   ", EventAt: s("2026-10-01")}
	if _, err := checkItem(model.JobTech, it, nil); err != nil {
		t.Errorf("dated event with blank url should pass: %v", err)
	}
}

func TestTruncateRuneSafe(t *testing.T) {
	got := truncate("가나다라마", 3)
	if got != "가나다…" {
		t.Errorf("truncate = %q", got)
	}
}
