// Command swingdesk is a lazydocker-style news desk for swing trading.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/hsk-kr/swingdesk/internal/config"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "swingdesk:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("swingdesk", flag.ContinueOnError)
	configFlag := fs.String("config", "", "path to config.yaml (default: XDG config dir)")
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

	fmt.Fprintf(out, "config:  %s\n", paths.ConfigFile)
	fmt.Fprintf(out, "data:    %s\n", paths.DataDir)
	fmt.Fprintf(out, "db:      %s\n", paths.DBFile)
	fmt.Fprintf(out, "inbox:   %s\n", paths.InboxDir)
	fmt.Fprintf(out, "runs:    %s\n", paths.RunsDir)
	fmt.Fprintf(out, "refresh: every %d min (%s)\n", cfg.RefreshMinutes, cfg.Timezone)
	return nil
}
