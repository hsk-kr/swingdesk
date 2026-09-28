-- 0001: initial swingdesk schema (mirrors docs/SCHEMA.sql without PRAGMAs, which are set per connection).


CREATE TABLE IF NOT EXISTS instruments (
  id            INTEGER PRIMARY KEY,
  symbol        TEXT    NOT NULL UNIQUE,
  name          TEXT    NOT NULL,
  kind          TEXT    NOT NULL CHECK (kind IN ('equity','cfd','index','commodity','other')),
  company_tag   TEXT,                    -- e.g. alphabet for GOOG+GOOGL
  notes         TEXT,                    -- CFD / private / dated future caveats
  enabled       INTEGER NOT NULL DEFAULT 1,
  sort_order    INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS refresh_runs (
  id            INTEGER PRIMARY KEY,
  started_at    TEXT    NOT NULL,        -- RFC3339
  finished_at   TEXT,
  status        TEXT    NOT NULL CHECK (status IN ('running','ok','partial','error')),
  error         TEXT,
  jobs_ok       INTEGER NOT NULL DEFAULT 0,
  jobs_fail     INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS items (
  id            INTEGER PRIMARY KEY,
  dedupe_key    TEXT    NOT NULL UNIQUE,
  run_id        INTEGER REFERENCES refresh_runs(id),
  instrument_id INTEGER REFERENCES instruments(id),
  category      TEXT    NOT NULL CHECK (category IN ('tech','market','news','opinion','event','valuation','other')),
  title         TEXT    NOT NULL,
  summary       TEXT    NOT NULL DEFAULT '',
  body          TEXT    NOT NULL DEFAULT '',
  source        TEXT,
  url           TEXT,
  published_at  TEXT,
  created_at    TEXT    NOT NULL,
  read_at       TEXT
);

CREATE INDEX IF NOT EXISTS idx_items_unread ON items(created_at DESC) WHERE read_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_items_instrument_unread ON items(instrument_id, created_at DESC) WHERE read_at IS NULL;

CREATE TABLE IF NOT EXISTS events (
  id            INTEGER PRIMARY KEY,
  instrument_id INTEGER REFERENCES instruments(id),
  item_id       INTEGER REFERENCES items(id),
  title         TEXT    NOT NULL,
  event_at      TEXT,                    -- RFC3339 date or datetime if known
  kind          TEXT    NOT NULL DEFAULT 'other' CHECK (kind IN ('earnings','product','macro','legal','other')),
  created_at    TEXT    NOT NULL
);

CREATE TABLE IF NOT EXISTS biases (
  id            INTEGER PRIMARY KEY,
  run_id        INTEGER REFERENCES refresh_runs(id),
  instrument_id INTEGER NOT NULL REFERENCES instruments(id),
  stance        TEXT    NOT NULL CHECK (stance IN ('long','short','none')),
  confidence    REAL    NOT NULL DEFAULT 0 CHECK (confidence >= 0 AND confidence <= 1),
  rationale     TEXT    NOT NULL DEFAULT '',
  created_at    TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_biases_latest ON biases(instrument_id, created_at DESC);
