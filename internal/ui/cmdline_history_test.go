package ui

import "testing"

// TestPushCmdHistoryDedupes verifies the recent-duplicate filter.
func TestPushCmdHistoryDedupes(t *testing.T) {
	e := &Explorer{cmdHistIdx: -1}
	e.pushCmdHistory("foo")
	e.pushCmdHistory("foo") // duplicate of last
	e.pushCmdHistory("bar")
	e.pushCmdHistory("bar") // duplicate of last
	e.pushCmdHistory("foo") // not adjacent → kept
	if got := len(e.cmdHistory); got != 3 {
		t.Fatalf("history length = %d, want 3 (got %v)", got, e.cmdHistory)
	}
	if e.cmdHistory[0] != "foo" || e.cmdHistory[1] != "bar" || e.cmdHistory[2] != "foo" {
		t.Errorf("history = %v, want [foo bar foo]", e.cmdHistory)
	}
}

// TestPushCmdHistoryIgnoresBlank checks empty / whitespace-only entries
// aren't added.
func TestPushCmdHistoryIgnoresBlank(t *testing.T) {
	e := &Explorer{cmdHistIdx: -1}
	e.pushCmdHistory("")
	e.pushCmdHistory("   ")
	e.pushCmdHistory("\t")
	if len(e.cmdHistory) != 0 {
		t.Errorf("blank inputs added to history: %v", e.cmdHistory)
	}
}

// TestPushCmdHistoryCaps trims the front when over the limit.
func TestPushCmdHistoryCaps(t *testing.T) {
	e := &Explorer{cmdHistIdx: -1}
	for i := 0; i < cmdHistoryMax+50; i++ {
		// Distinct entries (no consecutive dupes) so all get pushed.
		e.pushCmdHistory(string(rune('a' + (i % 26))))
	}
	if got := len(e.cmdHistory); got != cmdHistoryMax {
		t.Errorf("history length = %d, want %d", got, cmdHistoryMax)
	}
}
