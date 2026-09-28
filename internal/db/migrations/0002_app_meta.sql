-- 0002: key/value app state. `seeded_at` marks that the watchlist seed ran,
-- so deleting every instrument does not resurrect the seed.

CREATE TABLE IF NOT EXISTS app_meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

INSERT OR IGNORE INTO app_meta (key, value)
SELECT 'seeded_at', strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
WHERE EXISTS (SELECT 1 FROM instruments);
