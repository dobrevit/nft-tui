package ui

import (
	"testing"

	"github.com/dobrevit/nft-tui/internal/model"
)

func TestHumanRate(t *testing.T) {
	cases := []struct {
		rate float64
		unit string
		want string
	}{
		{0, "pps", "—"},
		{17, "pps", "17 pps"},
		{8423, "pps", "8.4K pps"},
		{1_200_000, "pps", "1.2M pps"},
		{4_500_000_000, "B/s", "4.5G B/s"},
	}
	for _, c := range cases {
		if got := humanRate(c.rate, c.unit); got != c.want {
			t.Errorf("humanRate(%v, %q) = %q, want %q", c.rate, c.unit, got, c.want)
		}
	}
}

func TestSortMonitorRows(t *testing.T) {
	// Synthesize a few rules with distinct counters.
	tbl := &model.Table{Family: "inet", Name: "filter"}
	ch := &model.Chain{Table: tbl, Name: "input"}
	mk := func(h uint64, dp, db uint64) *model.Rule {
		return &model.Rule{
			Chain:   ch,
			Handle:  h,
			Counter: model.Counter{Present: true, DeltaPackets: dp, DeltaBytes: db},
		}
	}
	rows := []monitorRow{
		{chain: ch, rule: mk(1, 10, 1000)},
		{chain: ch, rule: mk(2, 100, 100)},
		{chain: ch, rule: mk(3, 50, 50_000)},
	}

	e := &Explorer{monitorSort: sortByDeltaPkts}
	e.sortMonitorRows(rows)
	if rows[0].rule.Handle != 2 || rows[1].rule.Handle != 3 || rows[2].rule.Handle != 1 {
		t.Errorf("by Δpkts: got handles %d,%d,%d; want 2,3,1",
			rows[0].rule.Handle, rows[1].rule.Handle, rows[2].rule.Handle)
	}

	e.monitorSort = sortByBPS
	e.sortMonitorRows(rows)
	if rows[0].rule.Handle != 3 || rows[1].rule.Handle != 1 || rows[2].rule.Handle != 2 {
		t.Errorf("by bps: got handles %d,%d,%d; want 3,1,2",
			rows[0].rule.Handle, rows[1].rule.Handle, rows[2].rule.Handle)
	}
}
