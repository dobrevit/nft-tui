package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/dobrevit/nft-tui/internal/nft"
)

// deadmanPhase tracks where in the restore lifecycle we are.
type deadmanPhase int

const (
	dmIdle      deadmanPhase = iota // overlay hidden
	dmConfirm                       // showing "press Y to apply, Esc to cancel"
	dmApply                         // snapshotting + restoring (transient)
	dmCountdown                     // restored, countdown ticking
	dmRolling                       // rollback in flight
	dmDone                          // overlay hidden; final state in status bar
	dmError                         // rollback failed — overlay shows error
)

// deadmanWindow is how long the operator has to confirm after a
// restore is applied. Matches Cisco's `reload in 5` ergonomics scaled
// for nftables turnarounds.
const deadmanWindow = 60 * time.Second

// deadmanState carries the runtime state for the dead-man's switch.
// Pointer-only — held on Explorer.
type deadmanState struct {
	phase deadmanPhase
	// snapshotPath is the file the operator asked us to restore from.
	snapshotPath string
	// rollbackPath is the auto-snapshot of the live ruleset taken just
	// before the restore is applied. The 60-second timer (and the Esc
	// affordance) restore from this file.
	rollbackPath string
	deadline     time.Time

	confirmCh chan struct{}
	cancelCh  chan struct{}
	// sync.Once guards prevent a double-close panic when both the
	// operator and the timer fire near-simultaneously.
	confirmOnce sync.Once
	cancelOnce  sync.Once
}

// buildDeadmanPage assembles the centred overlay for the restore
// confirmation + countdown. One page, two visual states driven by
// e.deadman.phase.
func (e *Explorer) buildDeadmanPage() tview.Primitive {
	e.deadmanView = tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true)
	e.deadmanView.SetBorder(true).
		SetTitle(" Restore from snapshot ").
		SetTitleAlign(tview.AlignLeft)
	e.deadmanView.SetInputCapture(e.deadmanInputCapture)

	return tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(nil, 0, 1, false).
		AddItem(
			tview.NewFlex().SetDirection(tview.FlexRow).
				AddItem(nil, 0, 1, false).
				AddItem(e.deadmanView, 14, 0, true).
				AddItem(nil, 0, 1, false),
			0, 2, true,
		).
		AddItem(nil, 0, 1, false)
}

func (e *Explorer) deadmanInputCapture(ev *tcell.EventKey) *tcell.EventKey {
	switch e.deadman.phase {
	case dmConfirm:
		if ev.Key() == tcell.KeyEscape {
			e.closeDeadman("[yellow]restore cancelled[-]")
			return nil
		}
		if ev.Rune() == 'y' || ev.Rune() == 'Y' {
			e.beginRestore()
			return nil
		}
	case dmCountdown:
		if ev.Key() == tcell.KeyEscape {
			e.deadman.cancelOnce.Do(func() { close(e.deadman.cancelCh) })
			return nil
		}
		if ev.Rune() == 'y' || ev.Rune() == 'Y' {
			e.deadman.confirmOnce.Do(func() { close(e.deadman.confirmCh) })
			return nil
		}
	case dmError:
		if ev.Key() == tcell.KeyEscape {
			e.closeDeadman("[red]rollback errored — see audit dir for the rollback file[-]")
			return nil
		}
	}
	return ev
}

// requestRestore is the entry point: `:r <path>` lands here. Shows the
// confirmation overlay; nothing touches the kernel until the operator
// presses Y.
func (e *Explorer) requestRestore(snapshotPath string) {
	if e.committer == nil {
		e.setStatus("[red]restore requires --write[-]")
		return
	}
	if e.deadman == nil {
		e.deadman = &deadmanState{}
	}
	if e.deadman.phase != dmIdle && e.deadman.phase != dmDone {
		e.setStatus("[yellow]a restore is already in progress[-]")
		return
	}
	e.deadman = &deadmanState{
		phase:        dmConfirm,
		snapshotPath: snapshotPath,
	}
	e.renderDeadman()
	e.pages.ShowPage("deadman")
	e.app.SetFocus(e.deadmanView)
}

// beginRestore is called when the operator confirms the initial prompt.
// It snapshots the current ruleset to a rollback file, applies the
// requested restore, and arms the 60-second timer.
func (e *Explorer) beginRestore() {
	e.deadman.phase = dmApply
	e.renderDeadman()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Step 1: capture rollback. If this fails we MUST NOT proceed —
		// without a rollback file the dead-man's switch can't fire.
		rollback := filepath.Join(
			rollbackDir(e.committer),
			fmt.Sprintf("rollback-%s.nft", time.Now().Format("20060102-150405.000")),
		)
		if err := e.committer.Snapshot(ctx, rollback); err != nil {
			e.app.QueueUpdateDraw(func() {
				e.closeDeadman(fmt.Sprintf(
					"[red]restore aborted — could not capture rollback: %v[-]", err))
			})
			return
		}
		e.deadman.rollbackPath = rollback

		// Step 2: apply the restore.
		if err := e.committer.Restore(ctx, e.deadman.snapshotPath); err != nil {
			e.app.QueueUpdateDraw(func() {
				e.closeDeadman(fmt.Sprintf(
					"[red]restore failed: %v — kernel unchanged[-]", err))
			})
			return
		}
		_ = e.committer.Audit(nft.AuditEntry{
			Action: "restore",
			File:   e.deadman.snapshotPath,
		})

		// Step 3: arm the timer. The next interaction is via deadmanLoop.
		e.deadman.confirmCh = make(chan struct{})
		e.deadman.cancelCh = make(chan struct{})
		e.deadman.deadline = time.Now().Add(deadmanWindow)
		e.deadman.phase = dmCountdown
		e.app.QueueUpdateDraw(e.renderDeadman)
		go e.deadmanLoop()
	}()
}

// deadmanLoop is the single goroutine that owns transitions out of
// dmCountdown. It selects on confirm / cancel / tick; only ever one
// path runs to completion, which avoids races on the rollback fire.
func (e *Explorer) deadmanLoop() {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-e.deadman.confirmCh:
			e.app.QueueUpdateDraw(func() {
				e.deadman.phase = dmDone
				e.closeDeadman(fmt.Sprintf(
					"[green]restore confirmed — rollback file retained at %s[-]",
					e.deadman.rollbackPath))
				e.FullRebuild()
			})
			return
		case <-e.deadman.cancelCh:
			e.executeRollback("[yellow]restore cancelled — rolled back[-]")
			return
		case <-tick.C:
			if time.Now().After(e.deadman.deadline) {
				e.executeRollback("[yellow]dead-man's switch fired — auto-rolled back[-]")
				return
			}
			e.app.QueueUpdateDraw(e.renderDeadman)
		}
	}
}

// executeRollback runs `nft -f rollback.nft`. Called from the
// deadmanLoop goroutine — the Committer.Restore blocks while nft
// applies, which is fine since the UI thread is free.
func (e *Explorer) executeRollback(successMsg string) {
	e.deadman.phase = dmRolling
	e.app.QueueUpdateDraw(e.renderDeadman)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := e.committer.Restore(ctx, e.deadman.rollbackPath)
	if err == nil {
		_ = e.committer.Audit(nft.AuditEntry{
			Action: "rollback",
			File:   e.deadman.rollbackPath,
		})
	}
	e.app.QueueUpdateDraw(func() {
		if err != nil {
			e.deadman.phase = dmError
			e.renderDeadman()
			e.setStatus(fmt.Sprintf("[red]rollback FAILED: %v[-]", err))
			return
		}
		e.closeDeadman(successMsg)
		e.FullRebuild()
	})
}

// closeDeadman hides the overlay and surfaces msg on the status bar.
func (e *Explorer) closeDeadman(msg string) {
	if e.deadman != nil {
		e.deadman.phase = dmDone
	}
	e.pages.HidePage("deadman")
	e.app.SetFocus(e.tree)
	if msg != "" {
		e.setStatus(msg)
	}
}

// renderDeadman repaints the overlay according to the current phase.
func (e *Explorer) renderDeadman() {
	if e.deadman == nil {
		return
	}
	switch e.deadman.phase {
	case dmConfirm:
		e.deadmanView.SetText(fmt.Sprintf(
			"[::b]Restore the ruleset from a snapshot?[::-]\n\n"+
				"  source: %s\n\n"+
				"This will [red]flush every table[-] and then apply the file as a\n"+
				"single nft transaction. A rollback snapshot of the [::b]current[::-]\n"+
				"state is captured first; if you don't confirm within %s you\n"+
				"will be automatically rolled back.\n\n"+
				"  [yellow]Y[-] apply (with auto-rollback)        [yellow]Esc[-] cancel",
			e.deadman.snapshotPath, deadmanWindow))

	case dmApply:
		e.deadmanView.SetText(
			"[gray]applying restore — taking rollback snapshot, then nft -f…[-]")

	case dmCountdown:
		remaining := time.Until(e.deadman.deadline).Round(time.Second)
		if remaining < 0 {
			remaining = 0
		}
		bar := countdownBar(remaining, deadmanWindow)
		e.deadmanView.SetText(fmt.Sprintf(
			"[::b]Restore applied — confirm within %s or auto-rollback[::-]\n\n"+
				"  source:   %s\n"+
				"  rollback: %s\n\n"+
				"  %s   %s remaining\n\n"+
				"  [yellow]Y[-] keep new ruleset    [yellow]Esc[-] roll back now",
			deadmanWindow, e.deadman.snapshotPath, e.deadman.rollbackPath,
			bar, remaining))

	case dmRolling:
		e.deadmanView.SetText(
			"[gray]rolling back — applying rollback snapshot via nft -f…[-]")

	case dmError:
		e.deadmanView.SetText(fmt.Sprintf(
			"[red]ROLLBACK FAILED[-]\n\n"+
				"The dead-man's switch tried to restore the previous state but\n"+
				"nft -f returned an error. The rollback file is preserved at:\n\n"+
				"  %s\n\n"+
				"Investigate manually. Press [yellow]Esc[-] to dismiss this overlay.",
			e.deadman.rollbackPath))
	}
}

// countdownBar produces a fixed-width visual representation of the
// remaining time. Filled blocks shrink as the countdown advances.
func countdownBar(remaining, total time.Duration) string {
	const width = 40
	if total <= 0 {
		return ""
	}
	filled := int(float64(remaining) / float64(total) * float64(width))
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	colour := "[green]"
	switch {
	case remaining <= 5*time.Second:
		colour = "[red]"
	case remaining <= 15*time.Second:
		colour = "[yellow]"
	}
	bar := colour
	for i := 0; i < width; i++ {
		if i < filled {
			bar += "█"
		} else {
			bar += "░"
		}
	}
	bar += "[-]"
	return bar
}

// rollbackDir picks where to write rollback snapshots. Uses the
// Committer's audit dir if configured, else the OS temp dir. Falls
// back gracefully so a restore never fails purely on bad paths.
func rollbackDir(c *nft.Committer) string {
	if c != nil && c.AuditDir != "" {
		return c.AuditDir
	}
	return nft.DefaultAuditDir()
}
