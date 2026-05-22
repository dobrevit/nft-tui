//go:build integration

package nft

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dobrevit/nft-tui/internal/model"
	"github.com/dobrevit/nft-tui/internal/staged"
)

// TestIntegration_StagedOpsRoundTrip is the "every staged.Op renders
// to syntactically-valid nft" property check.
//
// For each concrete op type we:
//   1. Set up a fresh baseline (one table, one chain, one rule with
//      a known handle).
//   2. Build a representative op.
//   3. Call Committer.DryRun([]Op{op}).
//   4. Assert the dry-run accepted (nft -c -f exit 0).
//
// The check catches mismatches between our NFT() format strings and
// what nft actually accepts — the kind of bug where staged.NFT()
// happens to compile cleanly in Go but produces "rule" instead of
// "rule" plus the expected token order.
//
// Run with:
//
//	unshare -rn go test -tags=integration \
//	    -run=TestIntegration_StagedOpsRoundTrip -v ./internal/nft/
func TestIntegration_StagedOpsRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("nft"); err != nil {
		t.Skip("nft binary not on $PATH")
	}

	c := &Committer{}

	cases := []struct {
		name  string
		setup []string // extra mustNFT statements before the op runs
		op    func(handle uint64) staged.Op
	}{
		{
			name: "AddRule plain",
			op: func(uint64) staged.Op {
				return &staged.AddRule{
					Family: model.FamilyINet, Table: "filter", Chain: "input",
					Body: "tcp dport 80 accept",
				}
			},
		},
		{
			name: "AddRule with comment + escapes",
			op: func(uint64) staged.Op {
				return &staged.AddRule{
					Family: model.FamilyINet, Table: "filter", Chain: "input",
					Body:    `tcp dport 443 accept`,
					Comment: `https — "external"`, // tests our quote escaping
				}
			},
		},
		{
			name: "AddRule into a non-default chain",
			setup: []string{
				`add chain inet filter forward { type filter hook forward priority 0; }`,
			},
			op: func(uint64) staged.Op {
				return &staged.AddRule{
					Family: model.FamilyINet, Table: "filter", Chain: "forward",
					Body: `iifname "eth0" oifname "eth1" accept`,
				}
			},
		},
		{
			name: "InsertRule before handle",
			op: func(h uint64) staged.Op {
				return &staged.InsertRule{
					Family: model.FamilyINet, Table: "filter", Chain: "input",
					Position: h, Body: "ct state established,related accept",
				}
			},
		},
		{
			name: "InsertRule after handle",
			op: func(h uint64) staged.Op {
				return &staged.InsertRule{
					Family: model.FamilyINet, Table: "filter", Chain: "input",
					Position: h, After: true, Body: "ct state related accept",
				}
			},
		},
		{
			name: "ReplaceRule preserves handle",
			op: func(h uint64) staged.Op {
				return &staged.ReplaceRule{
					Family: model.FamilyINet, Table: "filter", Chain: "input",
					Handle: h, Body: "tcp dport 2222 accept",
				}
			},
		},
		{
			name: "ReplaceRule with comment",
			op: func(h uint64) staged.Op {
				return &staged.ReplaceRule{
					Family: model.FamilyINet, Table: "filter", Chain: "input",
					Handle: h, Body: "tcp dport 2222 accept",
					Comment: "post-replace",
				}
			},
		},
		{
			name: "DeleteRule by handle",
			op: func(h uint64) staged.Op {
				return &staged.DeleteRule{
					Family: model.FamilyINet, Table: "filter", Chain: "input",
					Handle: h,
				}
			},
		},
		{
			name: "FlushChain",
			op: func(uint64) staged.Op {
				return &staged.FlushChain{
					Family: model.FamilyINet, Table: "filter", Chain: "input",
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset baseline for each case so the seed-rule handle
			// is predictable and earlier cases don't leak state.
			mustNFT(t, "flush ruleset")
			mustNFT(t, "add table inet filter")
			mustNFT(t, "add chain inet filter input { type filter hook input priority 0; }")
			for _, s := range tc.setup {
				mustNFT(t, s)
			}
			mustNFT(t, "add rule inet filter input tcp dport 22 accept")

			handle := findInputDPort22Handle(t)
			op := tc.op(handle)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := c.DryRun(ctx, []staged.Op{op}); err != nil {
				t.Errorf("dry-run rejected %s:\n  rendered: %s\n  err:      %v",
					tc.name, op.NFT(), err)
			}
		})
	}
}

// findInputDPort22Handle looks up the kernel-assigned handle for the
// seed rule. We need this dynamically because nft assigns handles
// sequentially across the netns lifetime; the value isn't stable
// across test runs but IS stable within a single run after our
// baseline setup.
func findInputDPort22Handle(t *testing.T) uint64 {
	t.Helper()
	out, err := exec.Command("nft", "-a", "list", "chain", "inet", "filter", "input").CombinedOutput()
	if err != nil {
		t.Fatalf("nft -a list: %v\n%s", err, out)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, "dport 22") {
			continue
		}
		i := strings.LastIndex(line, "# handle ")
		if i < 0 {
			continue
		}
		h, err := strconv.ParseUint(strings.TrimSpace(line[i+len("# handle "):]), 10, 64)
		if err != nil {
			continue
		}
		return h
	}
	t.Fatalf("could not find handle for seed rule in:\n%s", out)
	return 0
}
