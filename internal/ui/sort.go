package ui

import "sort"

// ruleSortMode picks which column the rule list is ordered by. The
// natural mode preserves the kernel-evaluation order (the rules' own
// position in the chain), which is the DEFAULT — sorting by counter
// rearranges the display without changing rule semantics, and an
// operator who forgets that they sorted by pps could think the
// firewall evaluates rules in a different order than it actually
// does. The title bar surfaces an explicit indicator when sort is
// non-natural.
type ruleSortMode int

const (
	ruleSortNatural ruleSortMode = iota // by handle position (default)
	ruleSortByPackets
	ruleSortByBytes
)

// sortDisplayedRules orders e.displayedRules in place. Stable so
// ties preserve their natural order.
func (e *Explorer) sortDisplayedRules() {
	if e.ruleSort == ruleSortNatural && !e.ruleSortReverse {
		return // already in kernel-evaluation order, nothing to do
	}
	less := func(i, j int) bool {
		switch e.ruleSort {
		case ruleSortByPackets:
			return e.displayedRules[i].Counter.Packets > e.displayedRules[j].Counter.Packets
		case ruleSortByBytes:
			return e.displayedRules[i].Counter.Bytes > e.displayedRules[j].Counter.Bytes
		default: // ruleSortNatural with reverse on
			return e.displayedRules[i].Handle > e.displayedRules[j].Handle
		}
	}
	if e.ruleSortReverse {
		// Invert the comparator: ascending counters / chronological
		// (oldest handle first when reversed-natural).
		original := less
		less = func(i, j int) bool { return original(j, i) }
	}
	sort.SliceStable(e.displayedRules, less)
}

// cycleRuleSort moves to the next sort column. natural → packets →
// bytes → natural. Reverse is reset on column change so the new
// column starts in its default direction (descending for counters,
// ascending for natural).
func (e *Explorer) cycleRuleSort() {
	e.ruleSort = (e.ruleSort + 1) % 3
	e.ruleSortReverse = false
	if e.currentChain != nil {
		e.showChain(e.currentChain)
	}
	e.setStatus("sort: " + e.sortDescription())
}

// toggleRuleSortReverse flips the current sort direction.
func (e *Explorer) toggleRuleSortReverse() {
	e.ruleSortReverse = !e.ruleSortReverse
	if e.currentChain != nil {
		e.showChain(e.currentChain)
	}
	e.setStatus("sort: " + e.sortDescription())
}

// sortDescription is the short label shown in the rules title bar
// and in the status-bar toast on key press. "natural" + reverse is
// "natural ↑" to make clear it's not the kernel order.
func (e *Explorer) sortDescription() string {
	arrow := "↓"
	if e.ruleSortReverse {
		arrow = "↑"
	}
	switch e.ruleSort {
	case ruleSortByPackets:
		return "pps " + arrow
	case ruleSortByBytes:
		return "bps " + arrow
	}
	if e.ruleSortReverse {
		return "natural " + arrow + " (reverse of kernel order)"
	}
	return "natural"
}
