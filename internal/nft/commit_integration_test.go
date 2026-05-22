//go:build integration

// Integration tests for the commit/dry-run path. Require:
//   - a real `nft` binary on $PATH
//   - CAP_NET_ADMIN (or equivalent — typically run via `unshare -rn`)
//   - the kernel to support nftables
//
// Run with:
//
//	unshare -rn go test -tags=integration -run=Integration -v ./internal/nft/
//
// These tests mutate the kernel ruleset of the current network namespace.
// The fake-root netns from `unshare -rn` is the safe place to run them.

package nft

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/dobrevit/nft-tui/internal/model"
	"github.com/dobrevit/nft-tui/internal/staged"
)

func TestIntegration_DryRunAndCommit(t *testing.T) {
	if _, err := exec.LookPath("nft"); err != nil {
		t.Skip("nft binary not on $PATH")
	}

	// Establish a minimal baseline so add-rule has something to target.
	mustNFT(t, "flush ruleset")
	mustNFT(t, "add table inet filter")
	mustNFT(t, "add chain inet filter input { type filter hook input priority 0; }")

	auditDir := t.TempDir()
	c := &Committer{AuditDir: auditDir}

	// Bad dry-run: nft -c must reject a typo before any kernel mutation.
	bad := []staged.Op{
		&staged.AddRule{Family: model.FamilyINet, Table: "filter", Chain: "input", Body: "tcp dport 22 acccept"},
	}
	if err := c.DryRun(context.Background(), bad); err == nil {
		t.Fatal("DryRun on `acccept` typo unexpectedly succeeded")
	}

	// Good dry-run + commit.
	ops := []staged.Op{
		&staged.AddRule{
			Family: model.FamilyINet, Table: "filter", Chain: "input",
			Body: "tcp dport 9090 counter accept", Comment: "integration",
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := c.DryRun(ctx, ops); err != nil {
		t.Fatalf("DryRun on good ops failed: %v", err)
	}
	auditPath, err := c.Commit(ctx, ops)
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}
	if !strings.HasPrefix(auditPath, auditDir) {
		t.Errorf("audit path %q not under audit dir %q", auditPath, auditDir)
	}
	if _, err := os.Stat(auditPath); err != nil {
		t.Errorf("audit file missing: %v", err)
	}

	// The committed rule must be visible to `nft list`.
	out, err := exec.Command("nft", "list", "chain", "inet", "filter", "input").CombinedOutput()
	if err != nil {
		t.Fatalf("nft list failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "dport 9090") {
		t.Errorf("committed rule not visible:\n%s", out)
	}
	if !strings.Contains(string(out), `comment "integration"`) {
		t.Errorf("committed comment not visible:\n%s", out)
	}
}

// mustNFT runs `nft <stmt>` and fails the test on non-zero exit.
func mustNFT(t *testing.T, stmt string) {
	t.Helper()
	cmd := exec.Command("nft", strings.Fields(stmt)[0], strings.Join(strings.Fields(stmt)[1:], " "))
	// nft expects each top-level token as a separate argv; the cleanest
	// invocation for arbitrary statements is to pipe via stdin.
	cmd = exec.Command("nft", "-f", "-")
	cmd.Stdin = strings.NewReader(stmt + "\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("nft %q: %v\n%s", stmt, err, out)
	}
}
