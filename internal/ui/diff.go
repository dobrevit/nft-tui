package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/dobrevit/nft-tui/internal/staged"
)

// dryRunState tracks the outcome of the most recent `nft -c -f`
// invocation against the current staged ChangeList.
type dryRunState int

const (
	dryRunNotRun dryRunState = iota
	dryRunPass
	dryRunFail
)

// buildDiffPage assembles the staged-changes / diff page. Two stacked
// panes: a TextView listing the staged ops (with real before/after for
// ReplaceRule), and a TextView showing the raw nft script that will be
// passed to nft -f.
//
// F3 dry-runs, u unstages the selected op, U unstages everything, Esc
// returns to the explorer. F2 (commit) lands in Phase 3.3.
func (e *Explorer) buildDiffPage() tview.Primitive {
	e.diffSummary = tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(false)
	e.diffSummary.SetBorder(true).
		SetTitle(" Staged changes ").
		SetTitleAlign(tview.AlignLeft)

	e.diffScript = tview.NewTextView().
		SetDynamicColors(false).
		SetWrap(false)
	e.diffScript.SetBorder(true).
		SetTitle(" nft script (will be passed to nft -f) ").
		SetTitleAlign(tview.AlignLeft)

	e.diffStatus = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)

	footer := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft).
		SetText("[yellow]F2[-] commit (after F3 passes)   [yellow]F3[-] dry-run   [yellow]u[-] unstage last   [yellow]U[-] unstage all   [yellow]Esc[-] back")

	inner := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(e.diffSummary, 0, 2, true).
		AddItem(e.diffScript, 0, 3, false).
		AddItem(e.diffStatus, 1, 0, false).
		AddItem(footer, 1, 0, false)
	inner.SetBorder(true).
		SetTitle(" Diff & commit ").
		SetTitleAlign(tview.AlignLeft)
	inner.SetInputCapture(e.diffInputCapture)

	// Centre at ~80% width / 90% height.
	return tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(nil, 0, 1, false).
		AddItem(
			tview.NewFlex().SetDirection(tview.FlexRow).
				AddItem(nil, 0, 1, false).
				AddItem(inner, 0, 18, true).
				AddItem(nil, 0, 1, false),
			0, 9, true,
		).
		AddItem(nil, 0, 1, false)
}

func (e *Explorer) diffInputCapture(ev *tcell.EventKey) *tcell.EventKey {
	switch ev.Key() {
	case tcell.KeyEsc:
		e.closeDiff()
		return nil
	case tcell.KeyF2:
		e.commitStaged()
		return nil
	case tcell.KeyF3:
		e.runDryRun()
		return nil
	}
	switch ev.Rune() {
	case 'u':
		if _, ok := e.staged.Pop(); ok {
			e.dryRun = dryRunNotRun // staged list changed; previous result stale
			e.refreshDiff()
			e.refreshStatusBar(e.rs.FetchedAt)
		}
		return nil
	case 'U':
		e.staged.Clear()
		e.dryRun = dryRunNotRun
		e.refreshDiff()
		e.refreshStatusBar(e.rs.FetchedAt)
		return nil
	}
	return ev
}

// openDiff shows the diff page and repopulates it from the current
// staged ChangeList.
func (e *Explorer) openDiff() {
	e.refreshDiff()
	e.pages.ShowPage("diff")
	e.app.SetFocus(e.diffSummary)
}

// closeDiff returns to the main explorer.
func (e *Explorer) closeDiff() {
	e.pages.HidePage("diff")
	e.app.SetFocus(e.tree)
}

// refreshDiff repopulates the two panes from e.staged. Called on open
// and after any staged-list mutation.
func (e *Explorer) refreshDiff() {
	ops := e.staged.Ops()
	e.diffSummary.SetTitle(fmt.Sprintf(" Staged changes (%d) ", len(ops)))
	e.diffSummary.SetText(e.renderDiffSummary(ops))
	e.diffScript.SetText(renderNFTScript(ops))
	e.refreshDryRunStatus()
}

func (e *Explorer) refreshDryRunStatus() {
	switch e.dryRun {
	case dryRunNotRun:
		if e.staged.Len() == 0 {
			e.diffStatus.SetText("[gray]nothing staged[-]")
		} else {
			e.diffStatus.SetText("[gray]dry-run: not yet run — press F3 to validate[-]")
		}
	case dryRunPass:
		e.diffStatus.SetText("[green]dry-run: ✓ OK — safe to commit[-]")
	case dryRunFail:
		e.diffStatus.SetText("[red]dry-run: ✗ FAILED[-]\n" + e.dryRunErr)
	}
}

// renderDiffSummary produces the [::b]summary[::-] of staged ops with a
// real before/after for ReplaceRule by looking up the original rule via
// the explorer's ruleIdx (captured at the most recent read).
func (e *Explorer) renderDiffSummary(ops []staged.Op) string {
	if len(ops) == 0 {
		return "[gray](no staged changes — open the editor with `a` to add a rule)[-]"
	}
	var b strings.Builder
	for i, op := range ops {
		fmt.Fprintf(&b, "[::b][%d][::-] ", i+1)
		switch v := op.(type) {
		case *staged.AddRule:
			fmt.Fprintf(&b, "[green]+ add[-]   %s %s %s\n      %s",
				v.Family, v.Table, v.Chain, v.Body)
			if v.Comment != "" {
				fmt.Fprintf(&b, ` comment %q`, v.Comment)
			}
			b.WriteByte('\n')

		case *staged.InsertRule:
			fmt.Fprintf(&b, "[green]+ insert[-] %s %s %s @position %d\n      %s",
				v.Family, v.Table, v.Chain, v.Position, v.Body)
			b.WriteByte('\n')

		case *staged.DeleteRule:
			fmt.Fprintf(&b, "[red]- delete[-]  %s %s %s handle %d",
				v.Family, v.Table, v.Chain, v.Handle)
			if old := e.lookupRuleNFT(string(v.Family), v.Table, v.Chain, v.Handle); old != "" {
				fmt.Fprintf(&b, "\n      [red]was:[-] %s", old)
			}
			b.WriteByte('\n')

		case *staged.ReplaceRule:
			fmt.Fprintf(&b, "[yellow]~ replace[-] %s %s %s handle %d\n",
				v.Family, v.Table, v.Chain, v.Handle)
			if old := e.lookupRuleNFT(string(v.Family), v.Table, v.Chain, v.Handle); old != "" {
				fmt.Fprintf(&b, "      [red]- %s[-]\n", old)
			}
			fmt.Fprintf(&b, "      [green]+ %s[-]", v.Body)
			if v.Comment != "" {
				fmt.Fprintf(&b, ` comment %q`, v.Comment)
			}
			b.WriteByte('\n')

		case *staged.FlushChain:
			fmt.Fprintf(&b, "[red]flush[-]    %s %s %s\n",
				v.Family, v.Table, v.Chain)

		default:
			fmt.Fprintf(&b, "%s\n", op.Describe())
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// lookupRuleNFT returns the rendered NFT text of the rule at the
// supplied location, or "" if it's not in the current index (e.g. the
// rule was added in a previous staged op that hasn't been committed).
func (e *Explorer) lookupRuleNFT(family, table, chain string, handle uint64) string {
	key := fmt.Sprintf("%s|%s|%s|%d", family, table, chain, handle)
	if r, ok := e.ruleIdx[key]; ok {
		return r.NFT
	}
	return ""
}

// renderNFTScript serialises the staged ops as the file that would be
// passed to `nft -f`. Mirrors what internal/nft/commit.go writes.
func renderNFTScript(ops []staged.Op) string {
	if len(ops) == 0 {
		return "# (empty)\n"
	}
	var b strings.Builder
	for _, op := range ops {
		b.WriteString(op.NFT())
		b.WriteByte('\n')
	}
	return b.String()
}

// runDryRun invokes the Committer and stashes the result for display.
// Runs in a goroutine so the UI stays responsive against large rulesets;
// the result lands via QueueUpdateDraw.
func (e *Explorer) runDryRun() {
	if e.committer == nil {
		e.dryRun = dryRunFail
		e.dryRunErr = "no committer configured (start with --write)"
		e.refreshDryRunStatus()
		return
	}
	ops := e.staged.Ops()
	if len(ops) == 0 {
		e.dryRun = dryRunNotRun
		e.refreshDryRunStatus()
		return
	}

	e.diffStatus.SetText("[gray]dry-run: running…[-]")
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := e.committer.DryRun(ctx, ops)
		e.app.QueueUpdateDraw(func() {
			if err != nil {
				e.dryRun = dryRunFail
				e.dryRunErr = err.Error()
			} else {
				e.dryRun = dryRunPass
				e.dryRunErr = ""
			}
			e.refreshDryRunStatus()
		})
	}()
}

// commitStaged applies the staged ChangeList atomically via `nft -f`.
// Gated on a previously-passing dry-run so an operator can't fat-finger
// F2 onto an unvalidated buffer.
func (e *Explorer) commitStaged() {
	switch {
	case e.committer == nil:
		e.diffStatus.SetText("[red]no committer configured (start with --write)[-]")
		return
	case e.staged.Len() == 0:
		e.diffStatus.SetText("[gray]nothing staged to commit[-]")
		return
	case e.dryRun != dryRunPass:
		e.diffStatus.SetText(
			"[yellow]press F3 first — commit requires a passing dry-run[-]")
		return
	}

	ops := e.staged.Ops()
	e.diffStatus.SetText("[gray]committing…[-]")

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		auditPath, err := e.committer.Commit(ctx, ops)
		e.app.QueueUpdateDraw(func() {
			if err != nil {
				e.dryRun = dryRunFail
				e.dryRunErr = "commit failed: " + err.Error()
				e.refreshDryRunStatus()
				return
			}
			n := e.staged.Len()
			e.staged.Clear()
			e.dryRun = dryRunNotRun
			// Pull a fresh snapshot so the explorer reflects the new
			// state and the rule index includes any newly-assigned
			// handles.
			e.FullRebuild()
			e.closeDiff()
			msg := fmt.Sprintf("[green]committed %d change(s)[-]", n)
			if auditPath != "" {
				msg += fmt.Sprintf("  audit: %s", auditPath)
			}
			e.setStatus(msg)
		})
	}()
}
