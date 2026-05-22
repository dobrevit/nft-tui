package ui

import (
	"testing"

	"github.com/dobrevit/nft-tui/internal/model"
)

func ruleWithCounter(handle uint64, packets, bytes uint64) *model.Rule {
	return &model.Rule{
		Handle:  handle,
		Counter: model.Counter{Present: true, Packets: packets, Bytes: bytes},
	}
}

func handlesOf(rs []*model.Rule) []uint64 {
	out := make([]uint64, len(rs))
	for i, r := range rs {
		out[i] = r.Handle
	}
	return out
}

func equalSlice(a, b []uint64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestSortNaturalIsNoOp confirms the default sort never reorders —
// the kernel-evaluation order must survive untouched.
func TestSortNaturalIsNoOp(t *testing.T) {
	e := &Explorer{}
	e.displayedRules = []*model.Rule{
		ruleWithCounter(7, 100, 1_000),
		ruleWithCounter(3, 500, 100),
		ruleWithCounter(12, 50, 50_000),
	}
	e.sortDisplayedRules()
	if !equalSlice(handlesOf(e.displayedRules), []uint64{7, 3, 12}) {
		t.Errorf("natural sort changed order: %v", handlesOf(e.displayedRules))
	}
}

func TestSortByPackets(t *testing.T) {
	e := &Explorer{ruleSort: ruleSortByPackets}
	e.displayedRules = []*model.Rule{
		ruleWithCounter(7, 100, 0),
		ruleWithCounter(3, 500, 0),
		ruleWithCounter(12, 50, 0),
	}
	e.sortDisplayedRules()
	// Descending by packets: 500 → 100 → 50, i.e. handles 3, 7, 12.
	if got := handlesOf(e.displayedRules); !equalSlice(got, []uint64{3, 7, 12}) {
		t.Errorf("by-packets sort: got handles %v, want [3 7 12]", got)
	}
}

func TestSortByBytes(t *testing.T) {
	e := &Explorer{ruleSort: ruleSortByBytes}
	e.displayedRules = []*model.Rule{
		ruleWithCounter(7, 0, 1_000),
		ruleWithCounter(3, 0, 100),
		ruleWithCounter(12, 0, 50_000),
	}
	e.sortDisplayedRules()
	// Descending by bytes: 50000 → 1000 → 100, i.e. handles 12, 7, 3.
	if got := handlesOf(e.displayedRules); !equalSlice(got, []uint64{12, 7, 3}) {
		t.Errorf("by-bytes sort: got handles %v, want [12 7 3]", got)
	}
}

func TestSortReverseFlipsDirection(t *testing.T) {
	e := &Explorer{ruleSort: ruleSortByPackets, ruleSortReverse: true}
	e.displayedRules = []*model.Rule{
		ruleWithCounter(7, 100, 0),
		ruleWithCounter(3, 500, 0),
		ruleWithCounter(12, 50, 0),
	}
	e.sortDisplayedRules()
	// Ascending now: 50 → 100 → 500, i.e. handles 12, 7, 3.
	if got := handlesOf(e.displayedRules); !equalSlice(got, []uint64{12, 7, 3}) {
		t.Errorf("by-packets reversed: got handles %v, want [12 7 3]", got)
	}
}

func TestSortStableOnTies(t *testing.T) {
	e := &Explorer{ruleSort: ruleSortByPackets}
	e.displayedRules = []*model.Rule{
		ruleWithCounter(7, 100, 0),
		ruleWithCounter(3, 100, 0),
		ruleWithCounter(12, 100, 0),
	}
	e.sortDisplayedRules()
	// All packet counts equal; stable sort preserves input order.
	if got := handlesOf(e.displayedRules); !equalSlice(got, []uint64{7, 3, 12}) {
		t.Errorf("tied packets: got handles %v, want [7 3 12] (stable)", got)
	}
}

func TestSortDescription(t *testing.T) {
	cases := []struct {
		mode    ruleSortMode
		reverse bool
		want    string
	}{
		{ruleSortNatural, false, "natural"},
		{ruleSortNatural, true, "natural ↑ (reverse of kernel order)"},
		{ruleSortByPackets, false, "pps ↓"},
		{ruleSortByPackets, true, "pps ↑"},
		{ruleSortByBytes, false, "bps ↓"},
		{ruleSortByBytes, true, "bps ↑"},
	}
	for _, c := range cases {
		e := &Explorer{ruleSort: c.mode, ruleSortReverse: c.reverse}
		if got := e.sortDescription(); got != c.want {
			t.Errorf("mode=%v reverse=%v: got %q, want %q", c.mode, c.reverse, got, c.want)
		}
	}
}
