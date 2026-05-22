package ui

import (
	"fmt"
	"sort"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/dobrevit/nft-tui/internal/model"
)

// sortMetric chooses how the live monitor's top-N table is ordered.
type sortMetric int

const (
	sortByPPS sortMetric = iota
	sortByBPS
	sortByDeltaPkts
)

func (m sortMetric) String() string {
	switch m {
	case sortByPPS:
		return "pps"
	case sortByBPS:
		return "bps"
	case sortByDeltaPkts:
		return "Δpkts"
	}
	return "?"
}

// monitorRow holds one entry in the live-monitor table.
type monitorRow struct {
	chain *model.Chain
	rule  *model.Rule
}

// buildMonitorPage assembles the live-monitor page. It is hidden until
// the user presses `m` and refreshed on every ticker fire while
// visible (unless paused).
func (e *Explorer) buildMonitorPage() tview.Primitive {
	e.monitorTable = tview.NewTable().
		SetSelectable(true, false).
		SetFixed(1, 0)
	e.monitorTable.SetBorder(true).
		SetTitleAlign(tview.AlignLeft)

	footer := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft).
		SetText("[yellow]s[-] cycle sort   [yellow]p[-] pause/resume   [yellow]Esc[-] back")

	flex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(e.monitorTable, 0, 1, true).
		AddItem(footer, 1, 0, false)
	flex.SetInputCapture(e.monitorInputCapture)
	return flex
}

func (e *Explorer) monitorInputCapture(ev *tcell.EventKey) *tcell.EventKey {
	if ev.Key() == tcell.KeyEsc {
		e.closeMonitor()
		return nil
	}
	switch ev.Rune() {
	case 's':
		e.monitorSort = (e.monitorSort + 1) % 3
		e.refreshMonitor()
		return nil
	case 'p':
		e.monitorPaused = !e.monitorPaused
		e.refreshMonitor()
		return nil
	}
	return ev
}

func (e *Explorer) openMonitor() {
	e.refreshMonitor()
	e.pages.ShowPage("monitor")
	e.app.SetFocus(e.monitorTable)
}

func (e *Explorer) closeMonitor() {
	e.pages.HidePage("monitor")
	e.app.SetFocus(e.tree)
}

// monitorVisible reports whether the live-monitor page is the topmost.
// Called from applyRuleset so the table refreshes on each tick.
func (e *Explorer) monitorVisible() bool {
	name, _ := e.pages.GetFrontPage()
	return name == "monitor"
}

// refreshMonitor rebuilds the table from the current ruleset. Honours
// the paused flag — when paused, the table content is left as-is so
// the operator can read what was on screen at the moment they paused.
func (e *Explorer) refreshMonitor() {
	if e.monitorPaused {
		// Update the title only so the operator knows the freeze is intentional.
		e.monitorTable.SetTitle(e.monitorTitle(true))
		return
	}
	rows := e.collectMonitorRows()
	e.sortMonitorRows(rows)
	const top = 50
	if len(rows) > top {
		rows = rows[:top]
	}
	e.populateMonitorTable(rows)
}

func (e *Explorer) collectMonitorRows() []monitorRow {
	out := make([]monitorRow, 0, 64)
	for _, t := range e.rs.Tables {
		for _, c := range t.Chains {
			for _, r := range c.Rules {
				if !r.Counter.Present {
					continue
				}
				out = append(out, monitorRow{chain: c, rule: r})
			}
		}
	}
	return out
}

func (e *Explorer) sortMonitorRows(rows []monitorRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i].rule.Counter, rows[j].rule.Counter
		switch e.monitorSort {
		case sortByBPS:
			return a.DeltaBytes > b.DeltaBytes
		case sortByDeltaPkts:
			return a.DeltaPackets > b.DeltaPackets
		default: // sortByPPS — DeltaPackets is proportional to PPS when
			// elapsed is the same for all rows, so we order by Δpkts.
			return a.DeltaPackets > b.DeltaPackets
		}
	})
}

func (e *Explorer) populateMonitorTable(rows []monitorRow) {
	e.monitorTable.Clear()
	e.monitorTable.SetTitle(e.monitorTitle(false))

	headers := []string{"CHAIN", "H#", "PPS", "BPS", "ΔPKTS", "RULE"}
	for i, h := range headers {
		e.monitorTable.SetCell(0, i, headerCell(h))
	}
	if len(rows) == 0 {
		e.monitorTable.SetCell(1, 0,
			dataCell("[gray]<no counter-bearing rules — add `counter` to a rule to see traffic here>[-]"))
		return
	}
	elapsed := e.refreshDelta
	for i, mr := range rows {
		chainLabel := fmt.Sprintf("%s %s › %s",
			mr.chain.Table.Family, mr.chain.Table.Name, mr.chain.Name)
		e.monitorTable.SetCell(i+1, 0, dataCell(chainLabel))
		e.monitorTable.SetCell(i+1, 1, dataCell(fmt.Sprintf("%d", mr.rule.Handle)))
		e.monitorTable.SetCell(i+1, 2, dataCell(humanRate(mr.rule.Counter.PPS(elapsed), "pps")))
		e.monitorTable.SetCell(i+1, 3, dataCell(humanRate(mr.rule.Counter.BPS(elapsed), "B/s")))
		e.monitorTable.SetCell(i+1, 4, dataCell(humanCount(mr.rule.Counter.DeltaPackets)))
		e.monitorTable.SetCell(i+1, 5, dataCell(truncate(mr.rule.NFT, 70)))
	}
}

func (e *Explorer) monitorTitle(paused bool) string {
	t := fmt.Sprintf(" Live monitor — sort: %s   refresh: %s ",
		e.monitorSort, formatInterval(e.refreshDelta))
	if paused {
		t = " Live monitor — [yellow]PAUSED[-]   sort: " + e.monitorSort.String() + " "
	}
	return t
}

func formatInterval(d time.Duration) string {
	if d <= 0 {
		return "—"
	}
	return d.Round(100 * time.Millisecond).String()
}

// humanRate renders a per-second rate with a binary suffix and unit
// suffix (e.g. "1.2K pps", "62.0M B/s"). Designed for column-table
// display, so it pads to a constant width.
func humanRate(rate float64, unit string) string {
	switch {
	case rate >= 1e9:
		return fmt.Sprintf("%.1fG %s", rate/1e9, unit)
	case rate >= 1e6:
		return fmt.Sprintf("%.1fM %s", rate/1e6, unit)
	case rate >= 1e3:
		return fmt.Sprintf("%.1fK %s", rate/1e3, unit)
	case rate > 0:
		return fmt.Sprintf("%.0f %s", rate, unit)
	}
	return "—"
}
