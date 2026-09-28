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

	"github.com/hsk-kr/swingdesk/internal/sample"
	"github.com/hsk-kr/swingdesk/internal/ui"
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

type flags struct {
	config       string
	pathsOnly    bool
	insertSample bool
	ingestFile   string
	refreshOnce  bool
}

func parseFlags(args []string, out io.Writer) (flags, error) {
	var f flags
	fs := flag.NewFlagSet("swingdesk", flag.ContinueOnError)
	fs.SetOutput(out)
	fs.StringVar(&f.config, "config", "", "path to config.yaml (default: XDG config dir)")
	fs.BoolVar(&f.pathsOnly, "paths", false, "print resolved paths and exit")
	fs.BoolVar(&f.insertSample, "insert-sample", false, "insert placeholder inbox items and exit")
	fs.StringVar(&f.ingestFile, "ingest", "", "ingest one agent JSON file and exit")
	fs.BoolVar(&f.refreshOnce, "refresh-once", false, "run one refresh (agents + ingest) without the TUI and exit")
	if err := fs.Parse(args); err != nil {
		return flags{}, err
	}
	if fs.NArg() > 0 {
		return flags{}, fmt.Errorf("unexpected arguments: %v", fs.Args())
	}
	return f, nil
}

func run(args []string, out io.Writer, start startUI) error {
	f, err := parseFlags(args, out)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	a, err := openApp(context.Background(), f.config)
	if err != nil {
		return err
	}
	defer a.Close()

	ctx := context.Background()
	switch {
	case f.pathsOnly:
		a.printPaths(out)
		return nil
	case f.insertSample:
		n, err := sample.Insert(ctx, a.conn, time.Now())
		if err != nil {
			return fmt.Errorf("insert sample: %w", err)
		}
		fmt.Fprintf(out, "inserted %d sample items into %s\n", n, a.paths.DBFile)
		return nil
	case f.ingestFile != "":
		return a.ingestOne(ctx, out, f.ingestFile)
	case f.refreshOnce:
		return a.refreshOnce(ctx, out)
	}
	return a.runUI(start)
}
