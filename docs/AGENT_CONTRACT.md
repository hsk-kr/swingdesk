# Agent contract

Claude Code print-mode jobs must emit one JSON object matching this shape.
The Go ingest layer rejects files that fail validation.

## Envelope

```json
{
  "job": "names",
  "generated_at": "2026-09-27T11:41:00+01:00",
  "items": [
    {
      "symbol": "NVDA",
      "category": "news",
      "title": "Short factual headline",
      "summary": "2-4 sentences. What happened and why a swing trader cares.",
      "body": "Optional extra context. Still short.",
      "source": "Reuters",
      "url": "https://...",
      "published_at": "2026-09-27T10:00:00Z",
      "event_at": null,
      "event_kind": null
    }
  ],
  "biases": [
    {
      "symbol": "NVDA",
      "stance": "none",
      "confidence": 0.4,
      "rationale": "News is incremental. No fresh 3-10 day edge."
    }
  ]
}
```

## Rules

- `job` is `market`, `tech`, `names`, or `names_rest` (second names batch when `split_names` is on).
- `category` is one of `tech`, `market`, `news`, `opinion`, `event`, `valuation`, `other`.
- `stance` is `long`, `short`, or `none`.
- `confidence` is 0..1.
- Market/tech jobs may set `symbol` to `TECH100`, `CRUDE`, or omit it (`null`) for general tape items.
- Prefer primary sources. No item without a `url` unless it is a dated calendar event the agent can pin to an official IR page.
- Do not invent prices. This app is not a market-data terminal.
- Do not recommend position size. Stance is a research hint only.
- Skip stale stories older than ~72 hours unless they are still the live catalyst.
- Cap: 40 items per job file.

## Prompt skeleton (names job)

You are a research desk for a swing trader in Europe/London.
Horizon is 3 to 10 days. Not long-term investing. Not financial advice.

Watchlist (symbol, name, kind, notes):
{{watchlist_table}}

Now is {{now_rfc3339}}.

For each instrument, gather:
1. Material news from the last 24-48 hours
2. Upcoming dated events (earnings, product, legal, OPEC, etc.)
3. A terse read of recent expert/analyst tone if a public source exists
4. Anything that changes the swing setup (guidance, ban, outage, inventory)

Then emit one bias per enabled instrument: long, short, or none.
Sit out (`none`) when the tape is noise.

Use web search and fetch. Output only the JSON envelope.
