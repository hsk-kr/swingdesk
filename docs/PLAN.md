# swingdesk plan

## Product

A single-binary Go TUI that looks and feels like lazydocker: dark panels, left list + right detail, status bar, vim keys, mouse optional.

Purpose: stay current on tech + market + watchlist names so you can decide **long**, **short**, or **no trade** for swing holds. Not a broker. Not a signal service that auto-trades.

## Look and navigation (lazydocker-like)

```
┌─ swingdesk ── unread 14 ── last refresh 11:41 ── next 12:11 ── agents idle ──┐
│ WATCHLIST          │ INBOX (unread only)                         │
│ ▸ All              │ ▸ NVDA  news   Hopper H200 demand…          │
│   Tech             │   MSFT  event  Ignite keynote 21 Oct        │
│   Market           │   META  opinion Street cuts target…        │
│   AAPL             │   CRUDE news   Inventory build…             │
│   AMZN             │                                              │
│   …                │ DETAIL                                       │
│                    │ title / source / published                   │
│                    │ summary                                      │
│                    │ stance: short  conf 0.62                     │
│                    │ why it matters for a 3–10 day swing          │
└─ j/k move ─ enter open ─ r mark read ─ R refresh now ─ q quit ──┘
```

Panels:

1. Left: categories + instruments. Badge = unread count.
2. Center: unread items for the current filter, newest first.
3. Bottom or right: selected item body.
4. Footer: key hints, refresh countdown, agent/tmux status.

Keys (v1):

| Key | Action |
| --- | --- |
| `j` / `k` / arrows | move |
| `h` / `l` / tab | change pane |
| `enter` | focus detail / open URL later |
| `r` | mark selected item read |
| `a` | mark all visible read |
| `u` | undo last mark-read |
| `R` | force refresh now |
| `g` | jump top |
| `/` | filter inbox |
| `?` | help |
| `q` | quit |

Default filter: unread only. `s` toggles read items on (off by default).

## Stack

| Piece | Choice | Why |
| --- | --- | --- |
| Language | Go 1.23+ | requested |
| TUI | Bubble Tea + Bubbles + Lip Gloss | async ticks, subprocesses, and list/viewport widgets. Lazydocker itself uses gocui; we copy the *layout*, not the library. |
| DB | SQLite via `modernc.org/sqlite` (pure Go) | no CGO, easy single-file store |
| Scheduler | Bubble Tea `tea.Tick` every second for countdown; work fires on start + every 30 min | only while the process is alive |
| Agents | Claude Code CLI in a tmux session | requested |
| Model | omit `--model` so the CLI default is used (currently Opus 5.5) | requested |
| Config | YAML under `~/.config/swingdesk/config.yaml` | watchlist + intervals + tmux session name |
| Data dir | `~/.local/share/swingdesk/` | `swingdesk.db`, `inbox/`, `runs/` |

## Process model

```
you
 └─ terminal running `swingdesk`
      ├─ TUI event loop
      ├─ sqlite
      └─ agent runner
           └─ tmux session `swingdesk`
                ├─ pane macro    → claude -p … > inbox/run-id/macro.json
                ├─ pane tech     → …/tech.json
                └─ pane names    → …/names.json   (watchlist batched)
```

On refresh:

1. Insert a `refresh_runs` row (`running`).
2. Ensure tmux session `swingdesk` exists (`tmux new-session -d -s swingdesk`).
3. For each job, create or reuse a window, `send-keys` or `tmux new-window` the Claude command.
4. Agents must write **only** JSON files under the run directory. The Go app does not parse tmux pane text as the source of truth.
5. Watcher picks up completed JSON (file rename `*.json.tmp` → `*.json`, or a sibling `*.done` flag).
6. Upsert items with dedupe. Close the run (`ok` / `error` / `partial`).
7. TUI flashes "N new items".

Quit does not kill the tmux session by default (a late agent can still finish a file). Next launch imports any leftover inbox files. `kill_agents_on_quit: true` or `swingdesk -kill-agents` ends the session.

## Agent jobs (cost-aware)

Do **not** spawn one Opus session per ticker every 30 minutes. That is 14+ paid runs an hour.

v1 jobs per refresh (3 panes):

1. **market** — indices, rates, oil, risk-on/off, anything that moves `TECH100` and `CRUDE`.
2. **tech** — sector tape: AI infra, semis, cloud, ads, EVs.
3. **names** — the whole watchlist in one prompt. Per instrument: today's material news, next dated events, a short expert-consensus snapshot, and a swing bias.

If a names run starts timing out, set `split_names: true`: the names job becomes two batches (`names` = `megacap_symbols`, `names_rest` = the others), both from `prompts/names.md`.

Headless Claude invocation (print mode, structured output):

```bash
claude -p "$(cat prompts/names.md)" \
  --output-format json \
  --json-schema "$(cat prompts/schema.json)" \
  --permission-mode dontAsk \
  --allowedTools "WebSearch,WebFetch" \
  > "$INBOX/$RUN_ID/names.json.tmp" \
  && mv "$INBOX/$RUN_ID/names.json.tmp" "$INBOX/$RUN_ID/names.json"
```

Notes:

- Default model: do not pass `--model` unless config overrides it.
- Tools limited to web search/fetch so the agent researches instead of editing the repo (`--tools` + `--allowedTools`).
- Jobs run with `--safe-mode --strict-mcp-config --no-session-persistence`: no user CLAUDE.md, plugins, hooks or MCP servers leak into research runs.
- `dontAsk` avoids a stuck pane waiting for a human. If the local Claude Code build rejects that mode, fall back to a project `settings.json` allow-list for WebSearch/WebFetch only.
- `--dangerously-skip-permissions` is **not** the default. Document it as an explicit opt-in in config for people who accept the risk.
- tmux is still used so you can attach (`tmux attach -t swingdesk`) and watch the agents work, which is the point of "lazydocker + panes".

Each agent prompt must include:

- today's date and timezone (`Europe/London` default, configurable)
- the instrument table (symbol, name, kind)
- "this is for 3–10 day swing decisions, not long-term investing"
- "output JSON only matching the schema"
- "cite source URLs; skip rumors with no source"
- "bias is not financial advice; sit out when the tape is noise"

## Data model (see SCHEMA.sql)

Core tables: `instruments`, `items`, `events`, `biases`, `refresh_runs`, `read_state`.

`items` is the inbox. Categories: `tech`, `market`, `news`, `opinion`, `event`, `valuation`, `other`.

Dedupe key: `sha256(lower(symbol) + '|' + lower(title) + '|' + coalesce(canonical_url, ''))`. Same story arriving twice in 30 minutes is one row. If the summary changes but the key matches, update body/summary, keep `read_at`.

Mark-read writes `items.read_at`. Inbox query: `read_at IS NULL` plus optional instrument/category filter.

`biases` is the latest long/short/none call per instrument for the header line in detail view. History is kept (one row per run) so you can see if the desk flipped.

## Config sketch

```yaml
refresh_minutes: 30
timezone: Europe/London
tmux_session: swingdesk
claude_bin: claude
claude_model: ""          # empty = CLI default
claude_permission_mode: dontAsk
max_items_per_job: 40
data_dir: ""              # empty = XDG
# Instruments are not in config: they are seeded into SQLite from
# docs/WATCHLIST.yaml on first launch. See docs/config.example.yaml.
```

## Suggested repo layout when building starts

```
cmd/swingdesk/main.go
internal/ui/
internal/db/
internal/refresh/
internal/agent/          # tmux + claude command builder
internal/config/
internal/ingest/         # parse agent JSON → sqlite
prompts/market.md
prompts/tech.md
prompts/names.md
prompts/schema.json
docs/
```

## Implementation order

Match the GitHub issues:

1. Module skeleton, config, XDG paths
2. SQLite schema + migrations + seed watchlist
3. Bubble Tea shell that looks like lazydocker and renders fake items
4. Mark-read + unread-only query
5. Agent JSON contract + ingest + dedupe (fixture files, no Claude yet)
6. tmux + Claude runner + 30-minute scheduler
7. Wire live refresh into the TUI status bar
8. Polish: help modal, filter, undo, attach-hint

## Risks

- **Cost**: Opus every 30 minutes is expensive. Mitigation: 3 jobs not 14; hard cap items; opt-in `market_hours` window, per-job `claude_job_models` (e.g. haiku for tech headlines) and optional `claude_max_budget_usd`.
- **Claude Code not installed / not logged in**: TUI must show a clear error and keep showing existing DB rows.
- **tmux missing**: same — surface it, do not crash the inbox.
- **Stuck agents**: run timeout (e.g. 8 minutes). Mark run `partial`. Do not block the TUI.
- **Hallucinated news**: require URLs; UI shows source; user still has to open the link for anything they would size a trade on.
- **CFD quirks**: `SPCX` is not a public ticker. `SKHY` is a T212 symbol. `CRUDE` is a dated future. Prompts must pass `kind` + `notes` so the agent does not invent an NYSE listing.
- **Not financial advice**: README + footer + prompt. Bias field is a research hint, not an order.

## Open questions (defaults used until you override)

| Question | Default in this plan |
| --- | --- |
| Timezone | `Europe/London` |
| Treat GOOG + GOOGL as one name or two? | Two instruments, same company tag `alphabet` so news can attach to both |
| Refresh when the TUI is closed? | No, v1 is in-process only |
| Show read items? | Hidden by default; `s` toggles |
| Auto-open sources in browser? | Not v1; `c` copies the URL (OSC 52) |
| Market-hours-only refresh? | Off by default; `market_hours.enabled` |
| Kill tmux on quit? | No by default; `kill_agents_on_quit` |
