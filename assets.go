// Package swingdesk exposes files embedded from the repository root.
package swingdesk

import (
	"embed"
	_ "embed"
)

// WatchlistYAML is docs/WATCHLIST.yaml, the seed instrument list.
//
//go:embed docs/WATCHLIST.yaml
var WatchlistYAML []byte

// Prompts holds prompts/*.md templates and prompts/schema.json.
//
//go:embed prompts/*.md prompts/schema.json
var Prompts embed.FS
