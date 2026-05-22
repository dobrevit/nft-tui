//go:build integration

package nft

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestIntegration_WatchFiresOnExternalChange opens the Watch and then
// applies a rule via an external nft invocation. The watcher must
// deliver a ChangeEvent within a short timeout. Run with:
//
//	unshare -rn go test -tags=integration -run=TestIntegration_Watch -v ./internal/nft/
func TestIntegration_WatchFiresOnExternalChange(t *testing.T) {
	if _, err := exec.LookPath("nft"); err != nil {
		t.Skip("nft not on $PATH")
	}
	// Establish baseline so the rule add succeeds.
	mustNFT(t, "flush ruleset")
	mustNFT(t, "add table inet filter")
	mustNFT(t, "add chain inet filter input { type filter hook input priority 0; }")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eventCh, err := Watch(ctx, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	// Give the subscription a moment to settle, then mutate from outside.
	time.Sleep(100 * time.Millisecond)
	mustNFT(t, "add rule inet filter input tcp dport 8888 accept")

	select {
	case _, ok := <-eventCh:
		if !ok {
			t.Fatal("event channel closed before any event arrived")
		}
		// Good — got a coalesced notification.
	case <-time.After(2 * time.Second):
		t.Fatal("Watch did not deliver an event within 2 s of an external nft change")
	}
}

// TestIntegration_WatchCoalesces verifies a burst of changes collapses
// into far fewer events than the underlying count. Useful to confirm
// the debounce window doesn't fire on every NFT_MSG_NEWRULE.
func TestIntegration_WatchCoalesces(t *testing.T) {
	if _, err := exec.LookPath("nft"); err != nil {
		t.Skip("nft not on $PATH")
	}
	mustNFT(t, "flush ruleset")
	mustNFT(t, "add table inet filter")
	mustNFT(t, "add chain inet filter input { type filter hook input priority 0; }")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eventCh, err := Watch(ctx, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	// Apply 50 rules in a single nft -f transaction — one nft -f, many
	// NFT_MSG_NEWRULE events on the wire.
	var script strings.Builder
	for i := range 50 {
		script.WriteString("add rule inet filter input tcp dport ")
		script.WriteString("1000")
		script.WriteString(strings.Repeat("0", i%2)) // mild port variation
		script.WriteString(" accept\n")
		_ = i
	}
	// The above produces colliding ports; rewrite cleanly.
	script.Reset()
	for i := range 50 {
		script.WriteString("add rule inet filter input tcp dport ")
		fmtPort(&script, 20000+i)
		script.WriteString(" accept\n")
	}
	cmd := exec.Command("nft", "-f", "-")
	cmd.Stdin = strings.NewReader(script.String())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("nft -f burst: %v\n%s", err, out)
	}

	// Drain events for one second and count them.
	count := 0
	deadline := time.After(1 * time.Second)
loop:
	for {
		select {
		case <-eventCh:
			count++
		case <-deadline:
			break loop
		}
	}
	if count == 0 {
		t.Fatal("no events delivered for a 50-rule burst")
	}
	if count > 5 {
		t.Errorf("coalescing seems weak: %d events delivered for one transaction (want ≤5)", count)
	}
	t.Logf("delivered %d coalesced events for 50 underlying rule changes", count)
}

// fmtPort writes an integer to b without bringing in fmt just for this.
func fmtPort(b *strings.Builder, n int) {
	if n == 0 {
		b.WriteByte('0')
		return
	}
	var buf [10]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	b.Write(buf[i:])
}
