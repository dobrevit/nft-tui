// Command nft-tui is a terminal UI for inspecting and (eventually) editing
// the Linux nftables ruleset.
//
// Phase 2 ships read-only: connect via netlink, enumerate tables / chains /
// rules / sets, render them in a tview explorer. No edits, no commits.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/rivo/tview"

	"github.com/dobrevit/nft-tui/internal/model"
	"github.com/dobrevit/nft-tui/internal/nft"
	"github.com/dobrevit/nft-tui/internal/ui"
)

func main() {
	var (
		dumpOnly     = flag.Bool("dump", false, "fetch the ruleset, print a summary to stdout, and exit (no TUI)")
		refreshEvery = flag.Duration("refresh", 2*time.Second, "live-counter refresh interval (e.g. 500ms, 5s, 0 to disable)")
		writeMode    = flag.Bool("write", false, "enable edit/commit affordances (a / e / d keys, commit screen). Default is read-only.")
		auditDir     = flag.String("audit-dir", nft.DefaultAuditDir(), "directory where committed nft scripts are archived")
		useMonitor   = flag.Bool("monitor", true, "subscribe to kernel netlink events for immediate refresh on external changes")
	)
	flag.Parse()

	conn, err := nft.NewConn()
	if err != nil {
		fmt.Fprintf(os.Stderr, "nft-tui: %v\n", err)
		fmt.Fprintln(os.Stderr, "hint: nftables netlink usually requires CAP_NET_ADMIN.")
		fmt.Fprintln(os.Stderr, "      try: sudo ./nft-tui")
		fmt.Fprintln(os.Stderr, "      or, for dev without root: unshare -rn ./nft-tui")
		os.Exit(1)
	}
	defer conn.Close()

	rs, err := conn.ListRuleset()
	if err != nil {
		fmt.Fprintf(os.Stderr, "nft-tui: list ruleset: %v\n", err)
		os.Exit(1)
	}

	if *dumpOnly {
		dump(rs)
		return
	}

	app := tview.NewApplication()
	committer := &nft.Committer{AuditDir: *auditDir}
	exp := ui.NewExplorer(app, rs, conn.ListRuleset, *refreshEvery, *writeMode, committer)
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
