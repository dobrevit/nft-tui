package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// cmdKind identifies the action a `:`-typed string maps to.
type cmdKind int

const (
	// cmdSearch is the fallback: the input is a global-search query, not
	// a command. arg holds the original (un-trimmed) input.
	cmdSearch cmdKind = iota
	// cmdWrite snapshots the live ruleset to arg (a path). Triggered by
	// `:w <path>` or `:write <path>`.
	cmdWrite
	// cmdRead restores the live ruleset from arg (a path). Triggered by
	// `:r <path>` or `:restore <path>`. Wrapped in the dead-man's switch
	// by the UI (5.2); the parser itself is unaware.
	cmdRead
)

// command is the result of running parseCommand on the `:`-modal input.
type command struct {
	kind cmdKind
	arg  string
}

// parseCommand turns input into a command. The grammar is intentionally
// tiny: a verb token (with short and long forms), whitespace, then the
// remainder as the verb's argument. Anything that doesn't match a known
// verb is returned as cmdSearch — so existing search behaviour is
// preserved for free.
func parseCommand(input string) command {
	s := strings.TrimSpace(input)
	if s == "" {
		return command{kind: cmdSearch, arg: input}
	}
	if rest, ok := matchVerb(s, "w", "write"); ok {
		return command{kind: cmdWrite, arg: strings.TrimSpace(rest)}
	}
	if rest, ok := matchVerb(s, "r", "read", "restore"); ok {
		return command{kind: cmdRead, arg: strings.TrimSpace(rest)}
	}
	return command{kind: cmdSearch, arg: input}
}

// matchVerb returns (remainder, true) when s starts with any of verbs
// followed by whitespace or the end of the string. The remainder is the
// substring after the verb and the separating space.
func matchVerb(s string, verbs ...string) (string, bool) {
	for _, v := range verbs {
		if s == v {
			return "", true
		}
		if strings.HasPrefix(s, v+" ") || strings.HasPrefix(s, v+"\t") {
			return s[len(v)+1:], true
		}
	}
	return "", false
}

// expandTilde rewrites a leading `~/` (and bare `~`) to the operator's
// home directory. Leaves all other paths untouched.
func expandTilde(p string) string {
	switch {
	case p == "~":
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	case strings.HasPrefix(p, "~/"):
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

// cmdHistoryMax caps how many entries we retain. ~100 covers a single
// long session; older entries roll off the front.
const cmdHistoryMax = 100

// pushCmdHistory appends s to the recall buffer. Strips whitespace
// duplicates against the most-recent entry — repeatedly running the
// same `:` query shouldn't fill the buffer with one repeated string.
func (e *Explorer) pushCmdHistory(s string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return
	}
	if n := len(e.cmdHistory); n > 0 && e.cmdHistory[n-1] == s {
		e.cmdHistIdx = -1
		return
	}
	e.cmdHistory = append(e.cmdHistory, s)
	if len(e.cmdHistory) > cmdHistoryMax {
		e.cmdHistory = e.cmdHistory[len(e.cmdHistory)-cmdHistoryMax:]
	}
	e.cmdHistIdx = -1
}

// recallPrevCommand moves backwards through the history. The first Up
// from a fresh modal jumps to the most-recent entry; subsequent Ups
// walk further back.
func (e *Explorer) recallPrevCommand() {
	if len(e.cmdHistory) == 0 {
		return
	}
	switch {
	case e.cmdHistIdx == -1:
		e.cmdHistIdx = len(e.cmdHistory) - 1
	case e.cmdHistIdx > 0:
		e.cmdHistIdx--
	}
	e.searchInput.SetText(e.cmdHistory[e.cmdHistIdx])
}

// recallNextCommand moves forwards through the history. Past the most-
// recent entry, the input clears (matching readline / shell behaviour).
func (e *Explorer) recallNextCommand() {
	if len(e.cmdHistory) == 0 || e.cmdHistIdx == -1 {
		return
	}
	e.cmdHistIdx++
	if e.cmdHistIdx >= len(e.cmdHistory) {
		e.cmdHistIdx = -1
		e.searchInput.SetText("")
		return
	}
	e.searchInput.SetText(e.cmdHistory[e.cmdHistIdx])
}

// runCommand dispatches a parsed command to its handler. cmdSearch is a
// no-op here — the caller handles search via the existing path.
func (e *Explorer) runCommand(c command) {
	switch c.kind {
	case cmdWrite:
		e.runSnapshotCommand(c.arg)
	case cmdRead:
		e.runRestoreCommand(c.arg)
	}
}

// runSnapshotCommand writes the live ruleset to path via Committer.Snapshot.
// Synchronous because the typical ruleset takes <100 ms to dump.
func (e *Explorer) runSnapshotCommand(path string) {
	if path == "" {
		e.setStatus("[yellow]:w needs a path, e.g. `:w ~/snap.nft`[-]")
		return
	}
	if e.committer == nil {
		e.setStatus("[red]no committer configured (start with --write)[-]")
		return
	}
	target := expandTilde(path)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.committer.Snapshot(ctx, target); err != nil {
		e.setStatus(fmt.Sprintf("[red]snapshot failed: %v[-]", err))
		return
	}
	e.setStatus(fmt.Sprintf("[green]snapshot written to %s[-]", target))
}

// runRestoreCommand routes `:r <path>` into the dead-man's switch
// overlay (deadman.go). The overlay handles the apply, the rollback
// snapshot, the 60-second countdown, and the auto-rollback.
func (e *Explorer) runRestoreCommand(path string) {
	if path == "" {
		e.setStatus("[yellow]:r needs a path, e.g. `:r ~/snap.nft`[-]")
		return
	}
	e.requestRestore(expandTilde(path))
}
