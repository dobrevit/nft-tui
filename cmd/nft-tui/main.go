// Command nft-tui is a terminal UI for inspecting and managing the
// Linux nftables ruleset. See man/nft-tui.1 for the full reference.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/rivo/tview"

	"github.com/dobrevit/nft-tui/internal/config"
	"github.com/dobrevit/nft-tui/internal/model"
	"github.com/dobrevit/nft-tui/internal/nft"
	"github.com/dobrevit/nft-tui/internal/ui"
)

// Build-time identity. Populated via -ldflags by goreleaser /
// Makefile; defaults are useful when built ad-hoc by `go build`.
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	var (
		configPath   = flag.String("config", "", "config file path; default $XDG_CONFIG_HOME/nft-tui/config.toml if present")
		dumpOnly     = flag.Bool("dump", false, "fetch the ruleset, print a summary to stdout, and exit (no TUI)")
		refreshEvery = flag.Duration("refresh", 2*time.Second, "live-counter refresh interval (e.g. 500ms, 5s, 0 to disable)")
		writeMode    = flag.Bool("write", false, "enable edit/commit affordances (a / e / d keys, commit screen). Default is read-only.")
		auditDir     = flag.String("audit-dir", nft.DefaultAuditDir(), "directory where committed nft scripts are archived")
		useMonitor   = flag.Bool("monitor", true, "subscribe to kernel netlink events for immediate refresh on external changes")
		theme        = flag.String("theme", "default", "colour theme: "+ui.ThemeNames())
		columns      = flag.String("columns", "default", "rule-list column preset: "+ui.ColumnPresetNames())
		logFile      = flag.String("log-file", "", "append diagnostic logs to <path>; empty disables logging entirely (the TUI never writes to stderr)")
		showVersion  = flag.Bool("version", false, "print version information and exit")
	)
	flag.Parse()

	if *showVersion {
		fmt.Printf("nft-tui %s (commit %s, built %s)\n", version, commit, date)
		return
	}

	// Layer the per-host config file under the CLI flags. Anything the
	// operator set explicitly on argv wins; the config file fills in
	// the rest. A missing default file is silent.
	applyConfigDefaults(*configPath, refreshEvery, writeMode, auditDir, useMonitor, theme, columns, logFile)

	// Validate CLI / config combinations BEFORE opening the log file —
	// a bad theme name exits 2 without `defer closeLog()` getting to
	// run; not worth the gocritic warning when invalid-args errors
	// already go to stderr immediately. Logging covers what happens
	// AFTER we've committed to running.
	t, ok := ui.LookupTheme(*theme)
	if !ok {
		fmt.Fprintln(os.Stderr, ui.ThemeError(*theme))
		os.Exit(2)
	}
	t.Apply()

	colsIdx := ui.LookupColumnPreset(*columns)
	if colsIdx < 0 {
		fmt.Fprintf(os.Stderr, "nft-tui: unknown columns preset %q — choose one of: %s\n",
			*columns, ui.ColumnPresetNames())
		os.Exit(2)
	}

	// Configure structured logging. With no --log-file the slog
	// default is wired to io.Discard so well-meaning slog.Info calls
	// in any package can't disrupt the TUI by sneaking into stderr.
	closeLog := setupLogging(*logFile)
	defer closeLog()

	conn, err := nft.NewConn()
	if err != nil {
		// closeLog handles the deferred-skip vs os.Exit race — call
		// it explicitly so the "started" log entry gets flushed and
		// the file handle closes cleanly.
		closeLog()
		fmt.Fprintf(os.Stderr, "nft-tui: %v\n", err)
		fmt.Fprintln(os.Stderr, "hint: nftables netlink usually requires CAP_NET_ADMIN.")
		fmt.Fprintln(os.Stderr, "      try: sudo ./nft-tui")
		fmt.Fprintln(os.Stderr, "      or, for dev without root: unshare -rn ./nft-tui")
		os.Exit(1) //nolint:gocritic // closeLog above is the explicit cleanup
	}
	defer func() { _ = conn.Close() }()

	rs, err := conn.ListRuleset()
	if err != nil {
		// Two deferred resources to clean up explicitly because the
		// process exits before the runtime gets to either: the
		// netlink conn and the log file.
		_ = conn.Close()
		closeLog()
		fmt.Fprintf(os.Stderr, "nft-tui: list ruleset: %v\n", err)
		os.Exit(1) //nolint:gocritic // explicit cleanup above covers both defers
	}

	if *dumpOnly {
		dump(rs)
		return
	}

	app := tview.NewApplication()
	committer := &nft.Committer{AuditDir: *auditDir}
	exp := ui.NewExplorer(app, rs, conn.ListRuleset, *refreshEvery, *writeMode, committer, colsIdx)
	exp.StartRefresh()
	defer exp.StopRefresh()

	if *useMonitor {
		watchCtx, watchCancel := context.WithCancel(context.Background())
		defer watchCancel()
		eventCh, err := nft.Watch(watchCtx, 250*time.Millisecond)
		if err != nil {
			fmt.Fprintf(os.Stderr, "nft-tui: monitor disabled: %v\n", err)
		} else {
			go func() {
				for range eventCh {
					exp.TriggerRefresh()
				}
			}()
		}
	}
	if err := app.SetRoot(exp.Root(), true).EnableMouse(true).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "nft-tui: %v\n", err)
		os.Exit(1)
	}
}

// setupLogging configures slog according to --log-file. Empty path =>
// silent (everything written to io.Discard, but slog calls remain
// cheap). Path => append-mode file with a text handler at INFO level.
// Returns a deferred-close function the caller invokes on shutdown.
//
// stderr is NEVER a log target because the TUI owns the terminal —
// surprise stderr output would smear over the rendered UI.
func setupLogging(path string) (cleanup func()) {
	if path == "" {
		slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
		return func() {}
	}
	// 0o600: same permissions as the audit log; this file can contain
	// rule contents, set elements, and operator paths.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "nft-tui: --log-file %s: %v\n", path, err)
		os.Exit(2)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))
	slog.Info("nft-tui started",
		"version", version,
		"commit", commit,
		"pid", os.Getpid(),
	)
	return func() {
		slog.Info("nft-tui stopped")
		_ = f.Close()
	}
}

// applyConfigDefaults layers the per-host config file under any
// already-parsed CLI flags. An explicit flag wins (we use flag.Visit
// to know which were set on argv); a missing default config file is
// silent; a malformed one is fatal (exit 2) because applying half a
// config silently would be a surprise.
func applyConfigDefaults(
	cfgPath string,
	refresh *time.Duration,
	write *bool,
	auditDir *string,
	monitor *bool,
	theme, columns *string,
	logFile *string,
) {
	path := cfgPath
	pathExplicit := path != ""
	if path == "" {
		path = config.DefaultPath()
	}

	cfg, err := config.Load(path)
	if err != nil {
		// Surface a bad config loudly. A typo in the operator's
		// config silently falling back to defaults would be the
		// worst kind of bug.
		fmt.Fprintf(os.Stderr, "nft-tui: %v\n", err)
		os.Exit(2)
	}

	// If the operator pointed us at a specific file with --config,
	// and that file didn't exist, that's also an error (they meant
	// to use it). The "silent default" forgiveness only applies to
	// the XDG-default path.
	if pathExplicit {
		if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
			fmt.Fprintf(os.Stderr, "nft-tui: --config %s: file not found\n", path)
			os.Exit(2)
		}
	}

	explicit := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { explicit[f.Name] = true })

	if !explicit["refresh"] && cfg.Refresh != nil {
		*refresh = cfg.Refresh.AsDuration()
	}
	if !explicit["write"] && cfg.Write != nil {
		*write = *cfg.Write
	}
	if !explicit["audit-dir"] && cfg.AuditDir != nil {
		*auditDir = *cfg.AuditDir
	}
	if !explicit["monitor"] && cfg.Monitor != nil {
		*monitor = *cfg.Monitor
	}
	if !explicit["theme"] && cfg.Theme != nil {
		*theme = *cfg.Theme
	}
	if !explicit["columns"] && cfg.Columns != nil {
		*columns = *cfg.Columns
	}
	if !explicit["log-file"] && cfg.LogFile != nil {
		*logFile = *cfg.LogFile
	}
}

// dump prints a flat summary of the ruleset to stdout. Useful for smoke
// testing the read path without needing a working terminal (e.g. in a netns
// for development, or when piping the output to grep).
func dump(rs *model.Ruleset) {
	fmt.Printf("# ruleset fetched at %s — %d tables\n", rs.FetchedAt.Format("15:04:05"), len(rs.Tables))
	for _, t := range rs.Tables {
		fmt.Printf("table %s %s\n", t.Family, t.Name)
		for _, c := range t.Chains {
			if c.IsBase {
				fmt.Printf("  chain %s { type %s hook %s priority %d; policy %s; }\n",
					c.Name, c.Type, c.Hook, c.Priority, c.Policy)
			} else {
				fmt.Printf("  chain %s {}\n", c.Name)
			}
			for _, r := range c.Rules {
				if r.Counter.Present {
					fmt.Printf("    [%d] %s   # pkts=%d bytes=%d\n",
						r.Handle, r.NFT, r.Counter.Packets, r.Counter.Bytes)
				} else {
					fmt.Printf("    [%d] %s\n", r.Handle, r.NFT)
				}
			}
		}
		for _, s := range t.Sets {
			if s.IsMap {
				fmt.Printf("  map %s { type %s : %s; %d elements }\n",
					s.Name, s.KeyType, s.DataType, len(s.Elements))
			} else {
				fmt.Printf("  set %s { type %s; %d elements }\n",
					s.Name, s.KeyType, len(s.Elements))
			}
		}
	}
}
