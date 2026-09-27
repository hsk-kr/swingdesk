# swingdesk

A lazydocker-style terminal news desk for swing trading.

The TUI is a Go binary. On startup and every 30 minutes while it is running, it asks Claude Code agents (default model, currently Opus 5.5) living in tmux panes to gather:

- latest tech news
- broader stock-market news
- per-name research for a watchlist (today's news, upcoming events, expert opinions, company value / setup)

Results land in SQLite. The UI shows **unread items only**. You mark an item read with a keypress and it disappears from the inbox.

This repo is the **plan and issue tracker**. Implementation is not started yet.

## Why

Keep a tight loop for swing trades: read what changed, decide long / short / sit out, move on.

## Watchlist seed (from Trading 212 CFD screenshots, 27 Sep 2026)

Equities / single-name CFDs:

- Alphabet Class C `GOOG`
- Alphabet Class A `GOOGL`
- Amazon `AMZN`
- SK hynix `SKHY`
- SpaceX `SPCX` (private; CFD product, not a listed share)
- Apple `AAPL`
- AMD `AMD`
- Microsoft `MSFT`
- Dell `DELL`
- Tesla `TSLA`
- Meta `META`
- Nvidia `NVDA`

Macro / index / commodity:

- Crude Oil 16 Oct 2026 `CRUDE`
- USA Tech 100 `TECH100`

Prices in the screenshots are CFD quotes at capture time, not a data feed this app will scrape from T212.

## Status

Planning only. Follow the issues.

## Docs

- [docs/PLAN.md](docs/PLAN.md) — architecture, UI, scheduler, agents, schema, risks
- [docs/SCHEMA.sql](docs/SCHEMA.sql) — SQLite draft
- [docs/WATCHLIST.yaml](docs/WATCHLIST.yaml) — seed instruments
- [docs/AGENT_CONTRACT.md](docs/AGENT_CONTRACT.md) — Claude Code prompt + JSON shape

## Non-goals (v1)

- Placing trades or talking to a broker
- Scraping Trading 212
- Portfolio PnL
- Running 24/7 as a daemon when the TUI is closed (v1 only refreshes while the program is open)
