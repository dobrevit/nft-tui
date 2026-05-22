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
	"fmt"
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

	// Now exercise replace + delete. We need the handle of the rule we
	// just added; `nft -a` prints handles inline.
	handles := exec.Command("nft", "-a", "list", "chain", "inet", "filter", "input")
	hout, err := handles.CombinedOutput()
	if err != nil {
		t.Fatalf("nft -a list failed: %v\n%s", err, hout)
	}
	handle := extractHandle(string(hout), "dport 9090")
	if handle == 0 {
		t.Fatalf("could not find handle for the committed rule:\n%s", hout)
	}

	// Replace + delete in one commit.
	mutate := []staged.Op{
		&staged.ReplaceRule{
			Family: model.FamilyINet, Table: "filter", Chain: "input",
			Handle: handle, Body: "tcp dport 9091 counter accept",
			Comment: "replaced",
		},
	}
	if _, err := c.Commit(ctx, mutate); err != nil {
		t.Fatalf("replace commit failed: %v", err)
	}
	out, _ = exec.Command("nft", "list", "chain", "inet", "filter", "input").CombinedOutput()
	if strings.Contains(string(out), "dport 9090") {
		t.Errorf("old rule still present after replace:\n%s", out)
	}
	if !strings.Contains(string(out), "dport 9091") {
		t.Errorf("replacement rule not visible:\n%s", out)
	}

	// Delete by handle. The handle is preserved across a replace.
	del := []staged.Op{
		&staged.DeleteRule{
			Family: model.FamilyINet, Table: "filter", Chain: "input",
			Handle: handle,
		},
	}
	if _, err := c.Commit(ctx, del); err != nil {
		t.Fatalf("delete commit failed: %v", err)
	}
	out, _ = exec.Command("nft", "list", "chain", "inet", "filter", "input").CombinedOutput()
	if strings.Contains(string(out), "dport 9091") {
		t.Errorf("rule still present after delete:\n%s", out)
	}
}

// extractHandle scans `nft -a list chain` output for the line containing
// needle and returns the rule's handle. Returns 0 if not found.
func extractHandle(output, needle string) uint64 {
	for _, line := range strings.Split(output, "\n") {
		if !strings.Contains(line, needle) {
			continue
		}
		// Lines end with `# handle N` — split on `# handle ` and parse.
		i := strings.LastIndex(line, "# handle ")
		if i < 0 {
			continue
		}
		var h uint64
		if _, err := fmt.Sscanf(line[i:], "# handle %d", &h); err == nil {
			return h
		}
	}
	return 0
}

// TestIntegration_SnapshotRestore round-trips the ruleset: snapshot to
// disk, mutate the kernel, restore from the snapshot, verify the
// kernel state matches the snapshotted baseline.
func TestIntegration_SnapshotRestore(t *testing.T) {
	if _, err := exec.LookPath("nft"); err != nil {
		t.Skip("nft binary not on $PATH")
	}
	mustNFT(t, "flush ruleset")
	mustNFT(t, "add table inet filter")
	mustNFT(t, "add chain inet filter input { type filter hook input priority 0; }")
	mustNFT(t, "add rule inet filter input tcp dport 22 counter accept")

	c := &Committer{}
	snapPath := t.TempDir() + "/baseline.nft"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := c.Snapshot(ctx, snapPath); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	body, err := os.ReadFile(snapPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(body), "# nft-tui snapshot") {
		t.Errorf("snapshot missing header; got:\n%s", body)
	}
	if !strings.Contains(string(body), "tcp dport 22") {
		t.Errorf("snapshot missing rule:\n%s", body)
	}

	// Mutate the kernel — add a divergent rule.
	mustNFT(t, "add rule inet filter input tcp dport 9999 drop")
	out, _ := exec.Command("nft", "list", "chain", "inet", "filter", "input").CombinedOutput()
	if !strings.Contains(string(out), "dport 9999") {
		t.Fatalf("divergent rule did not stick:\n%s", out)
	}

	// Restore — the divergent rule must disappear, the original must stay.
	if err := c.Restore(ctx, snapPath); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	out, _ = exec.Command("nft", "list", "chain", "inet", "filter", "input").CombinedOutput()
	if strings.Contains(string(out), "dport 9999") {
		t.Errorf("post-restore: divergent rule still present:\n%s", out)
	}
	if !strings.Contains(string(out), "dport 22") {
		t.Errorf("post-restore: baseline rule missing:\n%s", out)
	}
}

// TestIntegration_RestoreRejectsInvalidSnapshot ensures the dry-run
// catches a malformed snapshot before the kernel is touched.
func TestIntegration_RestoreRejectsInvalidSnapshot(t *testing.T) {
	if _, err := exec.LookPath("nft"); err != nil {
		t.Skip("nft binary not on $PATH")
	}
	mustNFT(t, "flush ruleset")
	mustNFT(t, "add table inet filter")
	mustNFT(t, "add chain inet filter input { type filter hook input priority 0; }")
	mustNFT(t, "add rule inet filter input tcp dport 22 accept")

	bad := t.TempDir() + "/bogus.nft"
	if err := os.WriteFile(bad, []byte("this is not valid nft syntax\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c := &Committer{}
	if err := c.Restore(ctx, bad); err == nil {
		t.Fatal("Restore on garbage file unexpectedly succeeded")
	}
	// The original rule must still be present — kernel untouched.
	out, _ := exec.Command("nft", "list", "chain", "inet", "filter", "input").CombinedOutput()
	if !strings.Contains(string(out), "dport 22") {
		t.Errorf("kernel mutated despite dry-run failure:\n%s", out)
	}
}

// mustNFT runs `nft -f -` with stmt piped on stdin (so arbitrary nft
// commands round-trip without arg-vs-positional-token games) and
// fails the test on non-zero exit.
func mustNFT(t *testing.T, stmt string) {
	t.Helper()
	cmd := exec.Command("nft", "-f", "-")
	cmd.Stdin = strings.NewReader(stmt + "\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("nft %q: %v\n%s", stmt, err, out)
	}
}
