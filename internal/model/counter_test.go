package model

import (
	"testing"
	"time"
)

func TestCounterMergeAndRates(t *testing.T) {
	r := &Rule{Counter: Counter{Packets: 1000, Bytes: 100000, Present: true}}
	next := &Rule{Counter: Counter{Packets: 1100, Bytes: 110000, Present: true}}
	r.MergeCountersFrom(next)

	if r.Counter.Packets != 1100 || r.Counter.Bytes != 110000 {
		t.Errorf("absolute not updated: %+v", r.Counter)
	}
	if r.Counter.DeltaPackets != 100 || r.Counter.DeltaBytes != 10000 {
		t.Errorf("delta wrong: %+v", r.Counter)
	}

	// PPS over 2 s = 50.
	if got := r.Counter.PPS(2 * time.Second); got != 50 {
		t.Errorf("PPS(2s) = %v, want 50", got)
	}
	if got := r.Counter.BPS(2 * time.Second); got != 5000 {
		t.Errorf("BPS(2s) = %v, want 5000", got)
	}
	// PPS over zero elapsed is zero (first-tick guard).
	if got := r.Counter.PPS(0); got != 0 {
		t.Errorf("PPS(0) = %v, want 0", got)
	}
}

// Counter reset on the kernel side (rare but possible: `nft reset counter`)
// produces a "next" snapshot lower than the previous. The merge must
// clamp delta to 0 rather than wrap around uint64.
func TestCounterResetIsNotWrap(t *testing.T) {
	r := &Rule{Counter: Counter{Packets: 1_000_000, Bytes: 1_000_000_000}}
	next := &Rule{Counter: Counter{Packets: 500, Bytes: 50_000}}
	r.MergeCountersFrom(next)

	if r.Counter.DeltaPackets != 0 {
		t.Errorf("DeltaPackets after reset = %d, want 0 (clamp)", r.Counter.DeltaPackets)
	}
	if r.Counter.DeltaBytes != 0 {
		t.Errorf("DeltaBytes after reset = %d, want 0 (clamp)", r.Counter.DeltaBytes)
	}
}
