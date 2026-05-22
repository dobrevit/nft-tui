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
	"github.com/dobrevit/nft-tui/internal/nft"
	"github.com/dobrevit/nft-tui/internal/staged"
)

// Explorer is the main screen. Build it with NewExplorer, then call Root
// to get the widget to mount on the Application.
type Explorer struct {
	app *tview.Application
	rs  *model.Ruleset

	header *tview.TextView
	tree   *tview.TreeView
	detail *tview.Pages
	rules  *tview.Table
	info   *tview.TextView
	status *tview.TextView
	root   *tview.Flex

	// pages stacks the explorer body and modal-style overlays (search, help).
	pages *tview.Pages
	// rulesFlex wraps the rules Table with the filter InputField so the
	// filter can collapse without rebuilding the page.
	rulesFlex   *tview.Flex
	filterInput *tview.InputField
	filter      string

	// Search overlay (`:` modal).
	searchInput   *tview.InputField
	searchResults *tview.List

	// nodeByChain maps a chain into the TreeView so jumpToChain can find
	// the corresponding node. Refreshed on every refreshTree call.
	nodeByChain map[*model.Chain]*tview.TreeNode

	// displayedRules mirrors the rule rows currently in the rules Table
	// (post-filter), so a selected row index maps unambiguously to a rule
	// for actions like yank.
	displayedRules []*model.Rule

	// Phase 3 write-path state.
	writeMode bool
	staged    staged.ChangeList
	committer *nft.Committer

	// Editor widgets. Built once in build(), reused on every open.
	editorTitle   *tview.TextView
	editorBody    *tview.TextArea
	editorComment *tview.InputField
	editorPreview *tview.TextView
	editorMode    editorMode
	editorTarget  editorTarget

	// Diff/commit widgets.
	diffSummary   *tview.TextView
	diffScript    *tview.TextView
	diffStatus    *tview.TextView
	commitConfirm *tview.Modal
	dryRun        dryRunState
	dryRunErr     string

	// Live monitor.
	monitorTable  *tview.Table
	monitorSpark  *tview.TextView
	monitorSort   sortMetric
	monitorPaused bool

	// Restore / dead-man's switch state.
	deadmanView *tview.TextView
	deadman     *deadmanState

	// sparkBuffers is the per-rule ring buffer of pps samples used by
	// the monitor sparkline. Keyed by ruleKey; trimmed to sparkSamples
	// on every push. Entries for rules that disappear are pruned in
	// indexRules.
	sparkBuffers map[string][]float64

	host string

	// Refresh state.
	fetch    func() (*model.Ruleset, error)
	interval time.Duration
	stop     chan struct{}
	// refreshTrigger lets external sources (the netlink Watch from
	// Phase 4.3) request a fetch outside the ticker cadence. Buffer
	// of 1 collapses bursts into a single refresh.
	refreshTrigger chan struct{}

	// ruleIdx points to every Rule in the current Ruleset, keyed by
	// (family, table, chain, handle). Used to merge fresh counters in
	// place on each tick without disturbing tree state.
	ruleIdx map[string]*model.Rule

	// currentChain is the chain currently being shown in the right pane,
	// or nil if the right pane shows a Table/Set/empty info view. Tracked
	// so the refresh tick can re-render the visible rule table.
	currentChain *model.Chain

	// kernelDrift is set when a refresh detected a structural change that
	// the in-place merge couldn't apply. The status bar surfaces it; the
	// tree is rebuilt on the next refresh after the user opens a new node.
	kernelDrift bool

	// refreshDelta is the time between the most recent two successful
	// refreshes. Used by the live monitor and any view that needs to
	// convert Counter deltas into pps / bps.
	refreshDelta time.Duration
}

// NewExplorer builds the explorer screen against the supplied ruleset.
// app is used so global key handlers can call QueueUpdateDraw later.
// fetch and interval drive the live-counter refresh; pass a nil fetch
// to disable refresh (e.g. for tests or one-shot rendering).
// writeMode toggles the Phase 3 edit affordances (a / e / d keys, the
// rule editor page, the diff/commit screen). False means read-only.
// committer must be non-nil iff writeMode is true.
func NewExplorer(
	app *tview.Application,
	rs *model.Ruleset,
	fetch func() (*model.Ruleset, error),
	interval time.Duration,
	writeMode bool,
	committer *nft.Committer,
) *Explorer {
	e := &Explorer{
		app:       app,
		rs:        rs,
		host:      hostname(),
		fetch:     fetch,
		interval:  interval,
		writeMode: writeMode,
		committer: committer,
	}
	e.indexRules()
	e.build()
	return e
}

// indexRules rebuilds the (family, table, chain, handle) → *Rule index from
// the current Ruleset. Called after every full reload. Also prunes
// sparkBuffers entries for rules that no longer exist.
func (e *Explorer) indexRules() {
	e.ruleIdx = make(map[string]*model.Rule)
	for _, t := range e.rs.Tables {
		for _, c := range t.Chains {
			for _, r := range c.Rules {
				e.ruleIdx[ruleKey(r)] = r
			}
		}
	}
	if e.sparkBuffers != nil {
		for k := range e.sparkBuffers {
			if _, alive := e.ruleIdx[k]; !alive {
				delete(e.sparkBuffers, k)
			}
		}
	} else {
		e.sparkBuffers = make(map[string][]float64)
	}
}

// ruleKey returns the canonical key for a rule across refresh ticks.
// Stable as long as the rule's handle isn't reused (the kernel does not
// reuse handles within a chain's lifetime).
func ruleKey(r *model.Rule) string {
	return fmt.Sprintf("%s|%s|%s|%d",
		r.Chain.Table.Family, r.Chain.Table.Name, r.Chain.Name, r.Handle)
}

// StartRefresh launches the background ticker that polls for fresh
// counters. Safe to call once; subsequent calls are no-ops.
func (e *Explorer) StartRefresh() {
	if e.fetch == nil || e.interval <= 0 || e.stop != nil {
		return
	}
	e.stop = make(chan struct{})
	e.refreshTrigger = make(chan struct{}, 1)
	go e.refreshLoop()
}

// TriggerRefresh asks the refresh loop to fetch immediately. Safe to
// call from any goroutine. Non-blocking: if a refresh is already
// queued the call is a no-op.
func (e *Explorer) TriggerRefresh() {
	if e.refreshTrigger == nil {
		return
	}
	select {
	case e.refreshTrigger <- struct{}{}:
	default:
	}
}

// StopRefresh stops the background ticker. Idempotent.
func (e *Explorer) StopRefresh() {
	if e.stop == nil {
		return
	}
	close(e.stop)
	e.stop = nil
}

func (e *Explorer) refreshLoop() {
	t := time.NewTicker(e.interval)
	defer t.Stop()
	for {
		select {
		case <-e.stop:
			return
		case <-t.C:
			e.doRefresh()
		case <-e.refreshTrigger:
			e.doRefresh()
		}
	}
}

func (e *Explorer) doRefresh() {
	rs, err := e.fetch()
	e.app.QueueUpdateDraw(func() {
		if err != nil {
			e.setStatus(fmt.Sprintf("[red]refresh error: %v[-]", err))
			return
		}
		e.applyRuleset(rs)
	})
}

// applyRuleset merges a freshly-fetched ruleset into the live UI. If the
// structure matches (same rule handles, same chain layout), counters are
// updated in place — tree expansion, selection, and scroll positions are
// preserved. Structural changes flip kernelDrift on; the user can press
// `R` to do a full rebuild.
func (e *Explorer) applyRuleset(rs *model.Ruleset) {
	newRules := map[string]*model.Rule{}
	for _, t := range rs.Tables {
		for _, c := range t.Chains {
			for _, r := range c.Rules {
				newRules[ruleKey(r)] = r
			}
		}
	}

	structural := len(newRules) != len(e.ruleIdx)
	if !structural {
		for k := range newRules {
			if _, ok := e.ruleIdx[k]; !ok {
				structural = true
				break
			}
		}
	}

	if structural {
		e.kernelDrift = true
		e.refreshStatusBar(rs.FetchedAt)
		return
	}

	// In-place counter merge — keeps every pointer in e.rs valid and
	// updates DeltaPackets / DeltaBytes via the model helper.
	for k, old := range e.ruleIdx {
		if nr, ok := newRules[k]; ok {
			old.MergeCountersFrom(nr)
		}
	}
	e.refreshDelta = rs.FetchedAt.Sub(e.rs.FetchedAt)
	e.rs.FetchedAt = rs.FetchedAt
	e.recordSparkSamples()
	e.refreshHeader()
	e.refreshStatusBar(rs.FetchedAt)
	if e.currentChain != nil {
		e.showChain(e.currentChain)
	}
	if e.monitorVisible() {
		e.refreshMonitor()
	}
}

// FullRebuild reloads the ruleset from scratch and rebuilds the tree.
// Called from the `R` keybinding when kernelDrift is set, or manually by
// the user who suspects external changes.
func (e *Explorer) FullRebuild() {
	if e.fetch == nil {
		return
	}
	rs, err := e.fetch()
	if err != nil {
		e.setStatus(fmt.Sprintf("[red]rebuild failed: %v[-]", err))
		return
	}
	e.rs = rs
	e.currentChain = nil
	e.kernelDrift = false
	e.indexRules()
	e.refreshHeader()
	e.refreshTree()
	e.detail.SwitchToPage("info")
	e.info.SetText("[gray]Reloaded. Select a chain, table, or set on the left.[-]")
	e.refreshStatusBar(rs.FetchedAt)
}

func (e *Explorer) refreshHeader() {
	e.header.SetText(fmt.Sprintf(
		"[::b]nft-tui[::-]  host: %s   ruleset @ %s",
		e.host, e.rs.FetchedAt.Format("15:04:05"),
	))
}

func (e *Explorer) refreshStatusBar(fetched time.Time) {
	drift := ""
	if e.kernelDrift {
		drift = "  [yellow]kernel changed — press R to reload[-]"
	}
	mode := "RO"
	if e.writeMode {
		mode = "RW"
	}
	stagedHint := ""
	if e.staged.Len() > 0 {
		stagedHint = fmt.Sprintf("   [green]staged: %d[-]", e.staged.Len())
	}
	e.setStatus(fmt.Sprintf(
		"MODE: %s   refreshed %s%s%s   [q] quit  [?] help  [Tab] switch pane",
		mode, fetched.Format("15:04:05"), stagedHint, drift,
	))
}

// Root returns the top-level widget for mounting.
func (e *Explorer) Root() tview.Primitive { return e.root }

func (e *Explorer) build() {
	e.buildHeader()
	e.buildTree()
	e.buildDetail()
	e.buildStatus()

	body := tview.NewFlex().
		SetDirection(tview.FlexColumn).
		AddItem(e.tree, 32, 0, true).
		AddItem(e.detail, 0, 1, false)

	main := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(e.header, 1, 0, false).
		AddItem(body, 0, 1, true).
		AddItem(e.status, 1, 0, false)

	e.pages = tview.NewPages().
		AddPage("main", main, true, true).
		AddPage("search", e.buildSearchPage(), true, false).
		AddPage("help", e.buildHelpPage(), true, false).
		AddPage("editor", e.buildEditorPage(), true, false).
		AddPage("diff", e.buildDiffPage(), true, false).
		AddPage("monitor", e.buildMonitorPage(), true, false).
		AddPage("deadman", e.buildDeadmanPage(), true, false).
		AddPage("confirm-commit", e.buildCommitConfirm(), false, false)

	e.root = tview.NewFlex().SetDirection(tview.FlexRow).AddItem(e.pages, 0, 1, true)
	e.root.SetInputCapture(e.handleKey)
}

func (e *Explorer) buildHeader() {
	e.header = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	e.refreshHeader()
}

func (e *Explorer) buildTree() {
	e.tree = tview.NewTreeView().
		SetTopLevel(0).
		SetGraphics(true).
		SetGraphicsColor(tcell.ColorDarkGray)
	e.tree.SetBorder(true).SetTitle(" Ruleset ").SetTitleAlign(tview.AlignLeft)
	e.tree.SetChangedFunc(e.onTreeChange)
	e.tree.SetSelectedFunc(func(n *tview.TreeNode) {
		n.SetExpanded(!n.IsExpanded())
	})
	e.refreshTree()
}

// refreshTree rebuilds the TreeNode hierarchy from the current Ruleset
// and swaps it into the existing TreeView. The TreeView widget identity
// is preserved so its place in the layout is stable.
func (e *Explorer) refreshTree() {
	e.nodeByChain = make(map[*model.Chain]*tview.TreeNode)
	rootNode := tview.NewTreeNode(fmt.Sprintf("Ruleset (%d tables)", len(e.rs.Tables))).
		SetColor(tcell.ColorYellow).
		SetSelectable(false)
	for _, t := range e.rs.Tables {
		rootNode.AddChild(e.buildTableNode(t))
	}
	e.tree.SetRoot(rootNode).SetCurrentNode(rootNode)
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
		cnode := tview.NewTreeNode(label).
			SetReference(c).
			SetSelectable(true)
		e.nodeByChain[c] = cnode
		tnode.AddChild(cnode)
	}
	// Split into sets (key only) and maps (key→value) for clearer grouping.
	var plainSets, maps []*model.Set
	for _, s := range t.Sets {
		if s.IsMap {
			maps = append(maps, s)
		} else {
			plainSets = append(plainSets, s)
		}
	}
	if len(plainSets) > 0 {
		group := tview.NewTreeNode(fmt.Sprintf("sets (%d)", len(plainSets))).
			SetSelectable(false).
			SetColor(tcell.ColorGray)
		for _, s := range plainSets {
			group.AddChild(tview.NewTreeNode(
				fmt.Sprintf("%s : %s", s.Name, s.KeyType),
			).SetReference(s).SetSelectable(true))
		}
		tnode.AddChild(group)
	}
	if len(maps) > 0 {
		group := tview.NewTreeNode(fmt.Sprintf("maps (%d)", len(maps))).
			SetSelectable(false).
			SetColor(tcell.ColorGray)
		for _, s := range maps {
			group.AddChild(tview.NewTreeNode(
				fmt.Sprintf("%s : %s → %s", s.Name, s.KeyType, s.DataType),
			).SetReference(s).SetSelectable(true))
		}
		tnode.AddChild(group)
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

	e.filterInput = e.buildFilterInput()
	e.rulesFlex = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(e.rules, 0, 1, true).
		AddItem(e.filterInput, 0, 0, false)

	e.detail = tview.NewPages().
		AddPage("rules", e.rulesFlex, true, false).
		AddPage("info", e.info, true, true)
}

func (e *Explorer) buildStatus() {
	e.status = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	e.setStatus("MODE: RO   staged: 0   [q] quit  [?] help  [Tab] switch pane")
}

func (e *Explorer) handleKey(ev *tcell.EventKey) *tcell.EventKey {
	// When focus is in an input field, let it consume printable keys.
	focus := e.app.GetFocus()
	if focus == e.filterInput || focus == e.searchInput {
		return ev
	}

	if ev.Key() == tcell.KeyTab {
		if e.app.GetFocus() == e.tree {
			e.app.SetFocus(e.rules)
		} else {
			e.app.SetFocus(e.tree)
		}
		return nil
	}
	switch ev.Rune() {
	case 'q':
		e.app.Stop()
		return nil
	case 'R':
		e.FullRebuild()
		return nil
	case '/':
		e.showFilter()
		return nil
	case ':':
		e.searchInput.SetText("")
		e.searchResults.Clear()
		e.pages.ShowPage("search")
		e.app.SetFocus(e.searchInput)
		return nil
	case 'y':
		e.yankSelectedRule()
		return nil
	case 'a':
		if e.currentChain != nil {
			e.openEditorAdd(e.currentChain)
		} else {
			e.setStatus("[yellow]select a chain first to add a rule[-]")
		}
		return nil
	case 'e':
		e.editSelected()
		return nil
	case 'd':
		e.stageDeleteSelected()
		return nil
	case 'D':
		e.openDiff()
		return nil
	case 'm':
		e.openMonitor()
		return nil
	case '?':
		if name, _ := e.pages.GetFrontPage(); name == "help" {
			e.pages.HidePage("help")
			e.app.SetFocus(e.tree)
		} else {
			e.pages.ShowPage("help")
		}
		return nil
	}
	return ev
}

// yankSelectedRule writes the canonical nft syntax of the currently-
// selected rule to the system clipboard via OSC 52.
func (e *Explorer) yankSelectedRule() {
	r := e.selectedRule()
	if r == nil {
		e.setStatus("[yellow]no rule selected to yank[-]")
		return
	}
	if err := yankToTerminal(r.NFT); err != nil {
		e.setStatus(fmt.Sprintf("[red]yank failed: %v[-]", err))
		return
	}
	e.setStatus(fmt.Sprintf(
		"yanked rule #%d (%d bytes) — paste into your editor / config repo",
		r.Handle, len(r.NFT)))
}

// selectedRule returns the rule corresponding to the rules table's
// current selection, taking the active filter into account. Returns nil
// if there's no current chain or no selectable rule.
func (e *Explorer) selectedRule() *model.Rule {
	row, _ := e.rules.GetSelection()
	idx := row - 1 // skip header row
	if idx < 0 || idx >= len(e.displayedRules) {
		return nil
	}
	return e.displayedRules[idx]
}

// stageDeleteSelected appends a DeleteRule for the currently-selected
// rule and updates the status bar.
func (e *Explorer) stageDeleteSelected() {
	if !e.writeMode {
		e.setStatus("[yellow]read-only mode — start with --write to delete[-]")
		return
	}
	r := e.selectedRule()
	if r == nil {
		e.setStatus("[yellow]no rule selected to delete[-]")
		return
	}
	e.staged.Append(&staged.DeleteRule{
		Family: r.Chain.Table.Family,
		Table:  r.Chain.Table.Name,
		Chain:  r.Chain.Name,
		Handle: r.Handle,
	})
	e.refreshStatusBar(e.rs.FetchedAt)
	e.setStatus(fmt.Sprintf(
		"staged delete of rule #%d in %s %s %s — D to review, F3 dry-run, F2 commit",
		r.Handle, r.Chain.Table.Family, r.Chain.Table.Name, r.Chain.Name))
}

// editSelected opens the editor in modeEdit for the currently-selected
// rule.
func (e *Explorer) editSelected() {
	r := e.selectedRule()
	if r == nil {
		e.setStatus("[yellow]no rule selected to edit[-]")
		return
	}
	e.openEditorReplace(r)
}

func (e *Explorer) onTreeChange(node *tview.TreeNode) {
	if node == nil {
		return
	}
	switch ref := node.GetReference().(type) {
	case *model.Chain:
		e.currentChain = ref
		e.showChain(ref)
	case *model.Table:
		e.currentChain = nil
		e.showTable(ref)
	case *model.Set:
		e.currentChain = nil
		e.showSet(ref)
	default:
		e.currentChain = nil
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
	if e.filter != "" {
		title += fmt.Sprintf(" — filter: %q", e.filter)
	}
	e.rules.SetTitle(title)

	headers := []string{"H#", "PKTS", "BYTES", "IIF", "OIF", "SRC", "DST", "DPORT", "VERDICT", "RULE"}
	for col, h := range headers {
		e.rules.SetCell(0, col, headerCell(h))
	}

	e.displayedRules = e.displayedRules[:0]
	displayRow := 1
	matched := 0
	for _, r := range c.Rules {
		if !ruleMatches(r, e.filter) {
			continue
		}
		matched++
		e.displayedRules = append(e.displayedRules, r)
		e.rules.SetCell(displayRow, 0, dataCell(fmt.Sprintf("%d", r.Handle)))
		e.rules.SetCell(displayRow, 1, dataCell(humanCount(r.Counter.Packets)))
		e.rules.SetCell(displayRow, 2, dataCell(humanBytes(r.Counter.Bytes)))
		e.rules.SetCell(displayRow, 3, dataCell(blankDash(r.IIfName)))
		e.rules.SetCell(displayRow, 4, dataCell(blankDash(r.OIfName)))
		e.rules.SetCell(displayRow, 5, dataCell(blankDash(r.SAddr)))
		e.rules.SetCell(displayRow, 6, dataCell(blankDash(r.DAddr)))
		e.rules.SetCell(displayRow, 7, dataCell(blankDash(r.DPort)))
		e.rules.SetCell(displayRow, 8, dataCell(blankDash(r.Verdict)))
		e.rules.SetCell(displayRow, 9, dataCell(truncate(r.NFT, 80)))
		displayRow++
	}
	switch {
	case len(c.Rules) == 0:
		e.rules.SetCell(1, 0, dataCell("[gray]<empty>[-]"))
	case matched == 0:
		e.rules.SetCell(1, 0, dataCell("[gray]<no matches for filter>[-]"))
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
	kind := "set"
	if s.IsMap {
		kind = "map"
	}
	fmt.Fprintf(&b, "[::b]%s %s %s %s[::-]\n\n", kind, s.Table.Family, s.Table.Name, s.Name)
	fmt.Fprintf(&b, "key type:  %s\n", s.KeyType)
	if s.IsMap {
		fmt.Fprintf(&b, "data type: %s\n", s.DataType)
	}
	fmt.Fprintf(&b, "constant:  %v\n", s.Flags.Constant)
	fmt.Fprintf(&b, "dynamic:   %v\n", s.Flags.Dynamic)
	fmt.Fprintf(&b, "interval:  %v\n", s.Flags.Interval)
	fmt.Fprintf(&b, "counter:   %v\n", s.Flags.Counter)
	if s.Flags.Timeout {
		fmt.Fprintf(&b, "timeout:   %s\n", s.Timeout)
	}
	fmt.Fprintf(&b, "elements:  %d\n\n", len(s.Elements))

	count := 0
	emit := func(text string, timeoutLeft time.Duration, comment string) {
		writeSetElement(&b, text, timeoutLeft, comment)
		count++
	}

	if s.IsMap {
		for _, el := range s.Elements {
			if el.IntervalEnd {
				continue
			}
			val := el.Value
			if val == "" {
				val = "[gray]<missing-value>[-]"
			}
			emit(fmt.Sprintf("%s  →  %s", el.Key, val), el.TimeoutLeft, el.Comment)
			if count >= 200 {
				fmt.Fprintf(&b, "  [gray]… %d more elided[-]\n", len(s.Elements)-count)
				e.info.SetText(b.String())
				return
			}
		}
	} else {
		// Pair up interval-end sentinels for display.
		var pending string
		for _, el := range s.Elements {
			if el.IntervalEnd {
				if pending != "" {
					emit(pending+"-"+el.Key, 0, "")
					pending = ""
				}
				continue
			}
			if pending != "" {
				emit(pending, 0, "")
			}
			pending = el.Key
			emit(el.Key, el.TimeoutLeft, el.Comment)
			pending = ""
			if count >= 200 {
				fmt.Fprintf(&b, "  [gray]… %d more elided[-]\n", len(s.Elements)-count)
				e.info.SetText(b.String())
				return
			}
		}
		if pending != "" {
			emit(pending, 0, "")
		}
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
