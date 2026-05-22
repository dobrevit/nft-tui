package nft

import (
	"context"
	"fmt"
	"time"

	"github.com/google/nftables"
)

// ChangeEvent signals that the kernel ruleset has changed. Phase 4.3
// does not differentiate event types — the consumer always re-runs
// ListRuleset on receipt — but the type is here so we can specialise
// later (e.g. for per-set element updates) without breaking callers.
type ChangeEvent struct{}

// Watch opens a separate netlink subscription that fires a ChangeEvent
// whenever the kernel commits any nftables change (add/delete of
// tables / chains / rules / sets / set elements / objects).
//
// Bursts of events from a single transaction (e.g. one nft -f loading
// 200 rules) are coalesced via the debounce window: the returned
// channel emits once per quiet period of `coalesce` duration after
// the last underlying event.
//
// The returned channel is closed when ctx is done or when the netlink
// subscription drops. A separate google/nftables Conn is opened
// internally, so this does NOT share the read-path Conn's mutex; both
// can run concurrently without contention.
func Watch(ctx context.Context, coalesce time.Duration) (<-chan ChangeEvent, error) {
	conn, err := nftables.New()
	if err != nil {
		return nil, fmt.Errorf("open monitor netlink conn: %w", err)
	}
	mon := nftables.NewMonitor(
		nftables.WithMonitorAction(nftables.MonitorActionAny),
		nftables.WithMonitorObject(nftables.MonitorObjectAny),
		nftables.WithMonitorEventBuffer(64),
	)
	eventCh, err := conn.AddMonitor(mon)
	if err != nil {
		_ = mon.Close()
		_ = conn.CloseLasting()
		return nil, fmt.Errorf("add monitor: %w", err)
	}

	out := make(chan ChangeEvent, 1)
	go func() {
		defer close(out)
		defer mon.Close()
		defer conn.CloseLasting()

		var (
			pending     bool
			debounce    *time.Timer
			debounceCh  <-chan time.Time
		)
		armDebounce := func() {
			if debounce == nil {
				debounce = time.NewTimer(coalesce)
				debounceCh = debounce.C
				return
			}
			if !debounce.Stop() {
				select {
				case <-debounce.C:
				default:
				}
			}
			debounce.Reset(coalesce)
		}

		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-eventCh:
				if !ok {
					return
				}
				pending = true
				armDebounce()
			case <-debounceCh:
				if pending {
					// Non-blocking send: if the consumer hasn't drained
					// the previous notification yet, drop this one — it's
					// already implicit in the pending fetch.
					select {
					case out <- ChangeEvent{}:
					default:
					}
					pending = false
				}
				debounce = nil
				debounceCh = nil
			}
		}
	}()
	return out, nil
}
