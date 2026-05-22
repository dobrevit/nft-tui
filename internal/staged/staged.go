// Package staged represents pending edits to the kernel ruleset.
//
// Phase 3 of nft-tui never mutates the kernel directly from the editor;
// instead, each user action appends a StagedOp to a ChangeList. The diff
// view renders the list as nft syntax. The commit screen serialises the
// list to a temp file, runs `nft -c -f` to validate, then `nft -f` to
// apply atomically, and finally copies the file to an audit directory so
// the operator has a paste-ready record of what just happened.
//
// Why nft syntax instead of netlink batches:
//
//   - `nft -c` is a battle-tested validator; we get dry-run for free.
//   - The staged file is the audit log — paste it into Ansible or a
//     config repo.
//   - The raw-mode editor (F8) lets the user type nft directly; with
//     netlink writes we'd need our own nft-syntax parser.
//
// See docs/02-architecture.md for the full rationale.
package staged

import (
	"fmt"
	"strings"

	"github.com/dobrevit/nft-tui/internal/model"
)

// Op is the unit of staged work. Every concrete operation implements it.
type Op interface {
	// NFT renders the operation as one or more nft-syntax statements,
	// one per line, suitable for inclusion in a file fed to `nft -f`.
	NFT() string
	// Describe returns a short, human-friendly label for the staged-
	// changes view ("+ rule inet filter input", "- rule handle 17").
	Describe() string
	// Target returns the (family, table, chain) tuple this op acts on.
	// Used to group ops in the diff view and to know which ruleset
	// region to re-render after commit. ChainName may be empty for
	// table-level ops.
	Target() (family model.Family, table, chain string)
}

// ChangeList is an ordered set of staged operations.
type ChangeList struct {
	ops []Op
}

// Len returns the number of staged operations.
func (c *ChangeList) Len() int { return len(c.ops) }

// Ops returns a defensive copy of the staged operations in order.
func (c *ChangeList) Ops() []Op {
	out := make([]Op, len(c.ops))
	copy(out, c.ops)
	return out
}

// Append adds an op to the end of the list.
func (c *ChangeList) Append(op Op) { c.ops = append(c.ops, op) }

// Pop removes the last op (the undo affordance). Returns false if empty.
func (c *ChangeList) Pop() (Op, bool) {
	if len(c.ops) == 0 {
		return nil, false
	}
	last := c.ops[len(c.ops)-1]
	c.ops = c.ops[:len(c.ops)-1]
	return last, true
}

// Clear removes all staged ops.
func (c *ChangeList) Clear() { c.ops = nil }

// NFT serialises the entire list as a single nft script with one statement
// per line. Empty when nothing is staged.
func (c *ChangeList) NFT() string {
	if len(c.ops) == 0 {
		return ""
	}
	var b strings.Builder
	for i, op := range c.ops {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(op.NFT())
	}
	return b.String()
}

// --- concrete operations --------------------------------------------------

// AddRule appends a rule to the end of a chain. To insert at a specific
// position use InsertRule.
type AddRule struct {
	Family model.Family
	Table  string
	Chain  string
	// Body is the rule body in nft syntax — the part that goes after the
	// chain identifier in an `add rule …` statement. E.g. `tcp dport 22
	// counter accept`.
	Body string
	// Comment is appended as `comment "X"` if non-empty. Pass the
	// comment text only (no surrounding quotes).
	Comment string
}

func (o *AddRule) NFT() string {
	return fmt.Sprintf("add rule %s %s %s %s%s",
		o.Family, o.Table, o.Chain, o.Body, fmtComment(o.Comment))
}

func (o *AddRule) Describe() string {
	return fmt.Sprintf("+ rule %s %s %s   %s", o.Family, o.Table, o.Chain, o.Body)
}

func (o *AddRule) Target() (model.Family, string, string) {
	return o.Family, o.Table, o.Chain
}

// InsertRule places a rule at a specific position within a chain. position
// is the handle of the rule before which the new rule is inserted (nft's
// `insert rule … position H` actually inserts BEFORE handle H; nft's
// `add rule … position H` inserts AFTER. We use the former.)
type InsertRule struct {
	Family   model.Family
	Table    string
	Chain    string
	Position uint64
	Body     string
	Comment  string
}

func (o *InsertRule) NFT() string {
	return fmt.Sprintf("insert rule %s %s %s position %d %s%s",
		o.Family, o.Table, o.Chain, o.Position, o.Body, fmtComment(o.Comment))
}

func (o *InsertRule) Describe() string {
	return fmt.Sprintf("+ rule %s %s %s @position %d   %s",
		o.Family, o.Table, o.Chain, o.Position, o.Body)
}

func (o *InsertRule) Target() (model.Family, string, string) {
	return o.Family, o.Table, o.Chain
}

// DeleteRule removes a rule identified by its kernel handle.
type DeleteRule struct {
	Family model.Family
	Table  string
	Chain  string
	Handle uint64
}

func (o *DeleteRule) NFT() string {
	return fmt.Sprintf("delete rule %s %s %s handle %d",
		o.Family, o.Table, o.Chain, o.Handle)
}

func (o *DeleteRule) Describe() string {
	return fmt.Sprintf("- rule %s %s %s handle %d",
		o.Family, o.Table, o.Chain, o.Handle)
}

func (o *DeleteRule) Target() (model.Family, string, string) {
	return o.Family, o.Table, o.Chain
}

// ReplaceRule overwrites the body of a rule, preserving its handle.
type ReplaceRule struct {
	Family  model.Family
	Table   string
	Chain   string
	Handle  uint64
	Body    string
	Comment string
}

func (o *ReplaceRule) NFT() string {
	return fmt.Sprintf("replace rule %s %s %s handle %d %s%s",
		o.Family, o.Table, o.Chain, o.Handle, o.Body, fmtComment(o.Comment))
}

func (o *ReplaceRule) Describe() string {
	return fmt.Sprintf("~ rule %s %s %s handle %d   %s",
		o.Family, o.Table, o.Chain, o.Handle, o.Body)
}

func (o *ReplaceRule) Target() (model.Family, string, string) {
	return o.Family, o.Table, o.Chain
}

// FlushChain empties a chain in one shot. Distinct from deleting every
// rule individually because it survives even if rules were added by an
// external process between our last refresh and the commit.
type FlushChain struct {
	Family model.Family
	Table  string
	Chain  string
}

func (o *FlushChain) NFT() string {
	return fmt.Sprintf("flush chain %s %s %s", o.Family, o.Table, o.Chain)
}

func (o *FlushChain) Describe() string {
	return fmt.Sprintf("flush %s %s %s", o.Family, o.Table, o.Chain)
}

func (o *FlushChain) Target() (model.Family, string, string) {
	return o.Family, o.Table, o.Chain
}

// fmtComment returns ` comment "X"` for non-empty c, "" otherwise. The
// leading space is part of the return value so callers can concatenate
// unconditionally.
func fmtComment(c string) string {
	if c == "" {
		return ""
	}
	// nft accepts an unquoted token or a double-quoted string. Escaping
	// any embedded double quotes keeps the syntax valid.
	return ` comment "` + strings.ReplaceAll(c, `"`, `\"`) + `"`
}
