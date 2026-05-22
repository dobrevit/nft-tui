// Command nft-tui is a terminal UI for inspecting and (eventually) editing
// the Linux nftables ruleset.
//
// Phase 2 ships read-only: connect via netlink, enumerate tables / chains /
// rules / sets, render them in a tview explorer. No edits, no commits.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/rivo/tview"

	"github.com/dobrevit/nft-tui/internal/model"
	"github.com/dobrevit/nft-tui/internal/nft"
	"github.com/dobrevit/nft-tui/internal/ui"
)

func main() {
	var (
		dumpOnly = flag.Bool("dump", false, "fetch the ruleset, print a summary to stdout, and exit (no TUI)")
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
	exp := ui.NewExplorer(app, rs)
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
			fmt.Printf("  set %s { type %s; }\n", s.Name, s.KeyType)
		}
	}
}
