// Command swingdesk is a lazydocker-style news desk for swing trading.
package main

import (
	"context"
	"database/sql"
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
	"github.com/hsk-kr/swingdesk/internal/ingest"
	"github.com/hsk-kr/swingdesk/internal/model"
	"github.com/hsk-kr/swingdesk/internal/sample"
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
	insertSample := fs.Bool("insert-sample", false, "insert placeholder inbox items and exit")
	ingestFile := fs.String("ingest", "", "ingest one agent JSON file and exit")
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

	ctx := context.Background()
	conn, instruments, err := openAndSeed(ctx, paths.DBFile)
	if err != nil {
		return err
	}
	defer conn.Close()

	switch {
	case *pathsOnly:
		printPaths(out, cfg, paths, len(instruments))
		return nil
	case *insertSample:
		n, err := sample.Insert(ctx, conn, time.Now())
		if err != nil {
			return fmt.Errorf("insert sample: %w", err)
		}
		fmt.Fprintf(out, "inserted %d sample items into %s\n", n, paths.DBFile)
		return nil
	case *ingestFile != "":
		return ingestOne(ctx, out, conn, *ingestFile, cfg.MaxItemsPerJob)
	}

	return start(ui.New(ui.Options{
		Store:       db.NewStore(conn),
		Instruments: instruments,
		Location:    tz,
	}))
}

func ingestOne(ctx context.Context, out io.Writer, conn *sql.DB, path string, maxItems int) error {
	res, err := ingest.IngestFile(ctx, conn, path, ingest.Options{MaxItems: maxItems})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "%s: %d new, %d updated, %d events, %d biases, %d skipped\n",
		res.Job, res.Inserted, res.Updated, res.Events, res.Biases, len(res.Skipped))
	for _, s := range res.Skipped {
		fmt.Fprintf(out, "  skipped %s #%d %s: %s\n", s.Kind, s.Index, s.Symbol, s.Reason)
	}
	return nil
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
// launch, and returns the open connection plus the stored instruments.
func openAndSeed(ctx context.Context, dbFile string) (*sql.DB, []model.Instrument, error) {
	seed, err := watchlist.Parse(swingdesk.WatchlistYAML)
	if err != nil {
		return nil, nil, fmt.Errorf("embedded watchlist: %w", err)
	}
	conn, err := db.Open(ctx, dbFile)
	if err != nil {
		return nil, nil, err
	}
	if _, err := db.SeedInstruments(ctx, conn, seed); err != nil {
		conn.Close()
		return nil, nil, err
	}
	instruments, err := db.ListInstruments(ctx, conn)
	if err != nil {
		conn.Close()
		return nil, nil, err
	}
	return conn, instruments, nil
}
