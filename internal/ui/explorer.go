// Package ui owns the tview widgets and the per-screen wiring.
//
// The Explorer is the default screen: a left tree of families/tables/chains
// and a right pane that shows the contents of whatever is selected.
package ui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/dobrevit/nft-tui/internal/model"
)

// Explorer is the main screen. Build it with NewExplorer, then call Root
// to get the widget to mount on the Application.
type Explorer struct {
	app *tview.Application
	rs  *model.Ruleset

	tree   *tview.TreeView
	detail *tview.Pages
	rules  *tview.Table
	info   *tview.TextView
	status *tview.TextView
	root   *tview.Flex

	host string
}

// NewExplorer builds the explorer screen against the supplied ruleset.
// app is used so global key handlers can call QueueUpdateDraw later.
func NewExplorer(app *tview.Application, rs *model.Ruleset) *Explorer {
	e := &Explorer{
		app:  app,
		rs:   rs,
		host: hostname(),
	}
	e.build()
	return e
}

// Root returns the top-level widget for mounting.
func (e *Explorer) Root() tview.Primitive { return e.root }

func (e *Explorer) build() {
	header := e.buildHeader()
	e.buildTree()
	e.buildDetail()
	e.buildStatus()

	body := tview.NewFlex().
		SetDirection(tview.FlexColumn).
		AddItem(e.tree, 32, 0, true).
		AddItem(e.detail, 0, 1, false)

	e.root = tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(header, 1, 0, false).
		AddItem(body, 0, 1, true).
		AddItem(e.status, 1, 0, false)

	e.root.SetInputCapture(e.handleKey)
}

func (e *Explorer) buildHeader() *tview.TextView {
	h := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	h.SetText(fmt.Sprintf(
		"[::b]nft-tui[::-]  host: %s   ruleset @ %s",
		e.host, e.rs.FetchedAt.Format("15:04:05"),
	))
	return h
}

func (e *Explorer) buildTree() {
	rootNode := tview.NewTreeNode(fmt.Sprintf("Ruleset (%d tables)", len(e.rs.Tables))).
		SetColor(tcell.ColorYellow).
		SetSelectable(false)
	for _, t := range e.rs.Tables {
		rootNode.AddChild(e.buildTableNode(t))
	}

	e.tree = tview.NewTreeView().
		SetRoot(rootNode).
		SetCurrentNode(rootNode).
		SetTopLevel(0).
		SetGraphics(true).
		SetGraphicsColor(tcell.ColorDarkGray)
	e.tree.SetBorder(true).SetTitle(" Ruleset ").SetTitleAlign(tview.AlignLeft)
	e.tree.SetChangedFunc(e.onTreeChange)
	e.tree.SetSelectedFunc(func(n *tview.TreeNode) {
		n.SetExpanded(!n.IsExpanded())
	})
}

func (e *Explorer) buildTableNode(t *model.Table) *tview.TreeNode {
	tnode := tview.NewTreeNode(fmt.Sprintf("%s %s", t.Family, t.Name)).
		SetReference(t).
		SetSelectable(true).
		SetColor(tcell.ColorAqua)
	for _, c := range t.Chains {
		label := c.Name
		if c.IsBase {
			label = fmt.Sprintf("%s   {%s, %s, prio %d, %s}",
				c.Name, c.Type, c.Hook, c.Priority, c.Policy)
		}
		tnode.AddChild(tview.NewTreeNode(label).
			SetReference(c).
			SetSelectable(true))
	}
	if len(t.Sets) > 0 {
		sgroup := tview.NewTreeNode(fmt.Sprintf("sets (%d)", len(t.Sets))).
			SetSelectable(false).
			SetColor(tcell.ColorGray)
		for _, s := range t.Sets {
			sgroup.AddChild(tview.NewTreeNode(
				fmt.Sprintf("%s : %s", s.Name, s.KeyType),
			).SetReference(s).SetSelectable(true))
		}
		tnode.AddChild(sgroup)
	}
	return tnode
}

func (e *Explorer) buildDetail() {
	e.rules = tview.NewTable().
		SetSelectable(true, false).
		SetFixed(1, 0)
	e.rules.SetBorder(true).SetTitle(" Rules ").SetTitleAlign(tview.AlignLeft)

	e.info = tview.NewTextView().
		SetDynamicColors(true).
		SetWordWrap(true)
	e.info.SetBorder(true).SetTitle(" Info ").SetTitleAlign(tview.AlignLeft)

	e.detail = tview.NewPages().
		AddPage("rules", e.rules, true, false).
		AddPage("info", e.info, true, true)
}

func (e *Explorer) buildStatus() {
	e.status = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	e.setStatus("MODE: RO   staged: 0   [q] quit  [?] help  [Tab] switch pane")
}

func (e *Explorer) handleKey(ev *tcell.EventKey) *tcell.EventKey {
	if ev.Key() == tcell.KeyTab {
		if e.app.GetFocus() == e.tree {
			e.app.SetFocus(e.rules)
		} else {
			e.app.SetFocus(e.tree)
		}
		return nil
	}
	if ev.Rune() == 'q' {
		e.app.Stop()
		return nil
	}
	return ev
}

func (e *Explorer) onTreeChange(node *tview.TreeNode) {
	if node == nil {
		return
	}
	switch ref := node.GetReference().(type) {
	case *model.Chain:
		e.showChain(ref)
	case *model.Table:
		e.showTable(ref)
	case *model.Set:
		e.showSet(ref)
	default:
		e.detail.SwitchToPage("info")
		e.info.SetText("[gray]Select a chain, table, or set on the left.[-]")
	}
}

func (e *Explorer) showChain(c *model.Chain) {
	e.detail.SwitchToPage("rules")
	e.rules.Clear()
	title := fmt.Sprintf(" %s %s › %s ", c.Table.Family, c.Table.Name, c.Name)
	if c.IsBase {
		title = fmt.Sprintf(" %s %s › %s  [%s, %s, prio %d, policy %s] ",
			c.Table.Family, c.Table.Name, c.Name,
			c.Type, c.Hook, c.Priority, c.Policy)
	}
	e.rules.SetTitle(title)

	headers := []string{"H#", "PKTS", "BYTES", "IIF", "OIF", "SRC", "DST", "DPORT", "VERDICT", "RULE"}
	for col, h := range headers {
		e.rules.SetCell(0, col, headerCell(h))
	}

	for row, r := range c.Rules {
		e.rules.SetCell(row+1, 0, dataCell(fmt.Sprintf("%d", r.Handle)))
		e.rules.SetCell(row+1, 1, dataCell(humanCount(r.Counter.Packets)))
		e.rules.SetCell(row+1, 2, dataCell(humanBytes(r.Counter.Bytes)))
		e.rules.SetCell(row+1, 3, dataCell(blankDash(r.IIfName)))
		e.rules.SetCell(row+1, 4, dataCell(blankDash(r.OIfName)))
		e.rules.SetCell(row+1, 5, dataCell(blankDash(r.SAddr)))
		e.rules.SetCell(row+1, 6, dataCell(blankDash(r.DAddr)))
		e.rules.SetCell(row+1, 7, dataCell(blankDash(r.DPort)))
		e.rules.SetCell(row+1, 8, dataCell(blankDash(r.Verdict)))
		e.rules.SetCell(row+1, 9, dataCell(truncate(r.NFT, 80)))
	}
	if len(c.Rules) == 0 {
		e.rules.SetCell(1, 0, dataCell("[gray]<empty>[-]"))
	}
}

func (e *Explorer) showTable(t *model.Table) {
	e.detail.SwitchToPage("info")
	var b strings.Builder
	fmt.Fprintf(&b, "[::b]table %s %s[::-]\n\n", t.Family, t.Name)
	fmt.Fprintf(&b, "chains:     %d\n", len(t.Chains))
	fmt.Fprintf(&b, "sets:       %d\n", len(t.Sets))
	fmt.Fprintf(&b, "flowtables: %d\n", len(t.FlowTables))
	for _, c := range t.Chains {
		if c.IsBase {
			fmt.Fprintf(&b, "  • %-20s base   %s/%s prio=%d policy=%s\n",
				c.Name, c.Type, c.Hook, c.Priority, c.Policy)
		} else {
			fmt.Fprintf(&b, "  • %-20s regular\n", c.Name)
		}
	}
	e.info.SetText(b.String())
}

func (e *Explorer) showSet(s *model.Set) {
	e.detail.SwitchToPage("info")
	var b strings.Builder
	fmt.Fprintf(&b, "[::b]set %s %s %s[::-]\n\n", s.Table.Family, s.Table.Name, s.Name)
	fmt.Fprintf(&b, "key type:  %s\n", s.KeyType)
	fmt.Fprintf(&b, "constant:  %v\n", s.Flags.Constant)
	fmt.Fprintf(&b, "dynamic:   %v\n", s.Flags.Dynamic)
	fmt.Fprintf(&b, "interval:  %v\n", s.Flags.Interval)
	fmt.Fprintf(&b, "counter:   %v\n", s.Flags.Counter)
	if s.Flags.Timeout {
		fmt.Fprintf(&b, "timeout:   %s\n", s.Timeout)
	}
	fmt.Fprintf(&b, "elements:  %d\n\n", len(s.Elements))

	// Pair up interval-end sentinels for display.
	var pending string
	count := 0
	for _, el := range s.Elements {
		if el.IntervalEnd {
			if pending != "" {
				writeSetElement(&b, pending+"-"+el.Key, 0, "")
				pending = ""
			}
			continue
		}
		if pending != "" {
			writeSetElement(&b, pending, 0, "")
			count++
			if count >= 200 {
				fmt.Fprintf(&b, "  [gray]… %d more elided[-]\n", len(s.Elements)-count)
				return
			}
		}
		pending = el.Key
		// Carry timeout/comment forward to the actual emit, in case the
		// next element is a sentinel.
		writeSetElement(&b, el.Key, el.TimeoutLeft, el.Comment)
		pending = ""
		count++
		if count >= 200 {
			fmt.Fprintf(&b, "  [gray]… %d more elided[-]\n", len(s.Elements)-count)
			return
		}
	}
	if pending != "" {
		writeSetElement(&b, pending, 0, "")
	}
	e.info.SetText(b.String())
}

func writeSetElement(b *strings.Builder, key string, timeoutLeft time.Duration, comment string) {
	fmt.Fprintf(b, "  %s", key)
	if timeoutLeft > 0 {
		fmt.Fprintf(b, "   [gray]expires in %s[-]", timeoutLeft.Round(time.Second))
	}
	if comment != "" {
		fmt.Fprintf(b, "   [yellow]# %s[-]", comment)
	}
	b.WriteByte('\n')
}

func (e *Explorer) setStatus(s string) {
	e.status.SetText(s)
}

// --- small helpers ----------------------------------------------------------

func headerCell(s string) *tview.TableCell {
	return tview.NewTableCell(s).
		SetTextColor(tcell.ColorYellow).
		SetAttributes(tcell.AttrBold).
		SetSelectable(false).
		SetExpansion(1)
}

func dataCell(s string) *tview.TableCell {
	return tview.NewTableCell(s).SetExpansion(1)
}

func blankDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func humanCount(n uint64) string {
	switch {
	case n >= 1_000_000_000:
		return fmt.Sprintf("%.1fG", float64(n)/1e9)
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1e3)
	}
	return fmt.Sprintf("%d", n)
}

func humanBytes(n uint64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1fG", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1fM", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1fK", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%dB", n)
}

func hostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "?"
	}
	return h
}

// Refresh swaps in a freshly-fetched ruleset and rebuilds the tree.
// Phase 2 doesn't wire this yet, but having it here means the polling
// loop in Phase 2.1 will just call it.
func (e *Explorer) Refresh(rs *model.Ruleset) {
	e.rs = rs
	e.build()
}
