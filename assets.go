// Package swingdesk exposes files embedded from the repository root.
package swingdesk

import _ "embed"

// WatchlistYAML is docs/WATCHLIST.yaml, the seed instrument list.
//
//go:embed docs/WATCHLIST.yaml
var WatchlistYAML []byte
