# swingdesk

A lazydocker-style terminal news desk for swing trading.

The TUI is a Go binary. On startup and every 30 minutes while it is running, it asks Claude Code agents (default model) living in tmux windows to gather:

- latest tech news
- broader stock-market news
- per-name research for a watchlist (today's news, upcoming events, expert opinions, company value / setup)

Results land in SQLite. The UI shows **unread items only** by default (press `s` to include read items). You mark an item read with a keypress and it disappears from the inbox.

> Stances are a research hint, not financial advice.

## Requirements

- Go 1.26+ (build only; the binary is CGO-free)
- `tmux`
- [Claude Code](https://claude.com/claude-code) (`claude`), logged in

## Install and run

```sh
make build            # bin/swingdesk
./bin/swingdesk       # open the TUI (first refresh starts immediately)
```

Watch the agents work: `tmux attach -t swingdesk` (one window per job: market, tech, names).

Other entry points:

| Command | What it does |
| --- | --- |
| `swingdesk -refresh-once` | one headless refresh (agents + ingest), then exit |
| `swingdesk -insert-sample` | insert placeholder items so the UI is usable without agents |
| `swingdesk -ingest file.json` | ingest one agent JSON file |
| `swingdesk -paths` | print config / data paths |
| `swingdesk -kill-agents` | kill the agents' tmux session |
| `swingdesk -config path.yaml` | use a specific config file |

## Keys

| Key | Action |
| --- | --- |
| `j` / `k` / arrows | move |
| `h` / `l` / `tab` / `shift+tab` | change pane |
| `enter` / `esc` | open detail / back |
| `g` / `G` | top / bottom |
| `r` | mark selected read |
| `a` | mark all visible read (current filter and `/` query) |
| `u` | undo last mark-read |
| `s` | show / hide read items |
| `/` | filter by title or symbol (`enter` keeps, `esc` clears) |
| `c` | copy URL (OSC 52; also the tmux buffer when inside tmux) |
| `R` | refresh now (resets the 30 minute timer) |
| `?` | full keybinding help |
| `q` | quit (running agents keep going; their output is imported next launch) |

## Configuration

`~/.config/swingdesk/config.yaml` (or `$XDG_CONFIG_HOME`, or `$SWINGDESK_CONFIG`). Every key is optional; see [docs/config.example.yaml](docs/config.example.yaml).

Data lives in `~/.local/share/swingdesk/` (`swingdesk.db`, `inbox/`, `runs/`, `swingdesk.log`).

Cost controls (all off by default): `market_hours` (scheduled refreshes only during US/EU hours, optional premarket), `split_names` (names job in two batches: megacaps / rest), `claude_job_models` (e.g. `{tech: haiku}`), `kill_agents_on_quit`.

Agents run `claude -p` with only `WebSearch,WebFetch`, `--safe-mode`, `--permission-mode dontAsk` and no `--model` unless you set `claude_model`. `--dangerously-skip-permissions` is an explicit opt-in (`claude_dangerously_skip_permissions: true`).

## Watchlist seed (from Trading 212 CFD screenshots, 27 Sep 2026)

Equities / single-name CFDs: `GOOG`, `GOOGL`, `AMZN`, `SKHY` (SK hynix, T212 CFD symbol), `SPCX` (SpaceX, private; CFD product), `AAPL`, `AMD`, `MSFT`, `DELL`, `TSLA`, `META`, `NVDA`.

Macro / index / commodity: `CRUDE` (Crude Oil 16 Oct 2026), `TECH100` (USA Tech 100).

Seeded into SQLite on first launch from [docs/WATCHLIST.yaml](docs/WATCHLIST.yaml); later edits in the DB are kept.

## Docs

- [docs/PLAN.md](docs/PLAN.md) — architecture, UI, scheduler, agents, schema, risks
- [docs/SCHEMA.sql](docs/SCHEMA.sql) — SQLite schema (mirrors `internal/db/migrations`)
- [docs/WATCHLIST.yaml](docs/WATCHLIST.yaml) — seed instruments
- [docs/AGENT_CONTRACT.md](docs/AGENT_CONTRACT.md) — Claude Code prompt + JSON shape (`prompts/`)

## Non-goals (v1)

- Placing trades or talking to a broker
- Scraping Trading 212
- Portfolio PnL
- Running 24/7 as a daemon when the TUI is closed (v1 only refreshes while the program is open)
