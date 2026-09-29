-- 0003: record how many jobs a run started with, so late files and stale-run
-- reconciliation use that run's total even if split_names changes later.

ALTER TABLE refresh_runs ADD COLUMN jobs_total INTEGER NOT NULL DEFAULT 3;
