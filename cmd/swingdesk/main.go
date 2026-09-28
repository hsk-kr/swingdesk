// Command swingdesk is a lazydocker-style news desk for swing trading.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/hsk-kr/swingdesk"
	"github.com/hsk-kr/swingdesk/internal/config"
	"github.com/hsk-kr/swingdesk/internal/db"
	"github.com/hsk-kr/swingdesk/internal/model"
	"github.com/hsk-kr/swingdesk/internal/ui"
	"github.com/hsk-kr/swingdesk/internal/watchlist"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, runProgram); err != nil {
		fmt.Fprintln(os.Stderr, "swingdesk:", err)
		os.Exit(1)
	}
}

// startUI runs the TUI until the user quits.
type startUI func(ui.Model) error

func runProgram(m ui.Model) error {
	_, err := tea.NewProgram(m).Run()
	return err
}

func run(args []string, out io.Writer, start startUI) error {
	fs := flag.NewFlagSet("swingdesk", flag.ContinueOnError)
	fs.SetOutput(out)
	configFlag := fs.String("config", "", "path to config.yaml (default: XDG config dir)")
	pathsOnly := fs.Bool("paths", false, "print resolved paths and exit")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %v", fs.Args())
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home dir: %w", err)
	}
	loc := config.ConfigPath(*configFlag, os.Getenv, home)
	cfg, err := config.Load(loc)
	if err != nil {
		return err
	}
	paths := config.ResolvePaths(cfg, loc.Path, os.Getenv, home)
	tz, err := cfg.Location()
	if err != nil {
		return fmt.Errorf("timezone: %w", err)
	}

	instruments, err := openAndSeed(context.Background(), paths.DBFile)
	if err != nil {
		return err
	}
	if *pathsOnly {
		printPaths(out, cfg, paths, len(instruments))
		return nil
	}

	now := time.Now()
	return start(ui.New(ui.Options{
		Instruments: instruments,
		Items:       ui.FixtureItems(instruments, now),
		Biases:      ui.FixtureBiases(instruments, now),
		Location:    tz,
	}))
}

func printPaths(out io.Writer, cfg config.Config, paths config.Paths, instruments int) {
	fmt.Fprintf(out, "config:  %s\n", paths.ConfigFile)
	fmt.Fprintf(out, "data:    %s\n", paths.DataDir)
	fmt.Fprintf(out, "db:      %s\n", paths.DBFile)
	fmt.Fprintf(out, "inbox:   %s\n", paths.InboxDir)
	fmt.Fprintf(out, "runs:    %s\n", paths.RunsDir)
	fmt.Fprintf(out, "refresh: every %d min (%s)\n", cfg.RefreshMinutes, cfg.Timezone)
	fmt.Fprintf(out, "instruments: %d\n", instruments)
}

// openAndSeed opens the DB, applies migrations, seeds the watchlist on first
// launch, and returns the stored instruments.
func openAndSeed(ctx context.Context, dbFile string) ([]model.Instrument, error) {
	seed, err := watchlist.Parse(swingdesk.WatchlistYAML)
	if err != nil {
		return nil, fmt.Errorf("embedded watchlist: %w", err)
	}
	conn, err := db.Open(ctx, dbFile)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if _, err := db.SeedInstruments(ctx, conn, seed); err != nil {
		return nil, err
	}
	return db.ListInstruments(ctx, conn)
}
