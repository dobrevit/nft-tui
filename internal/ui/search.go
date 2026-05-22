package ui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/dobrevit/nft-tui/internal/model"
)

// ruleMatches reports whether a rule contains query as a case-insensitive
// substring in any human-relevant field. Empty query matches everything.
//
// Reads from r.SearchKey, a lower-cased haystack precomputed at read time;
// the per-call cost is one ToLower of the (short) query plus a single
// strings.Contains. Falls back to a slow path if SearchKey wasn't built
// (synthetic rules in tests).
func ruleMatches(r *model.Rule, query string) bool {
	if query == "" {
		return true
	}
	q := strings.ToLower(query)
	if r.SearchKey != "" {
		return strings.Contains(r.SearchKey, q)
	}
	// Slow fallback for rules without a precomputed key.
	for _, h := range [...]string{
		fmt.Sprintf("%d", r.Handle),
		r.NFT, r.Comment,
		r.IIfName, r.OIfName,
		r.SAddr, r.DAddr,
		r.SPort, r.DPort,
		r.Verdict,
	} {
		if strings.Contains(strings.ToLower(h), q) {
			return true
		}
	}
	return false
}

// searchHit names a single rule that matched a global search.
type searchHit struct {
	chain *model.Chain
	rule  *model.Rule
}

// searchRuleset returns every rule whose ruleMatches the query, in
// breadth-first order (table → chain → rule). Capped at maxResults to
// keep the modal usable even on huge rulesets.
func searchRuleset(rs *model.Ruleset, query string, maxResults int) []searchHit {
	if query == "" || rs == nil {
		return nil
	}
	out := make([]searchHit, 0, 64)
	for _, t := range rs.Tables {
		for _, c := range t.Chains {
			for _, r := range c.Rules {
				if ruleMatches(r, query) {
					out = append(out, searchHit{chain: c, rule: r})
					if len(out) >= maxResults {
						return out
					}
				}
			}
		}
	}
	return out
}

// --- Filter input (anchored at the bottom of the rules page) --------------

// buildFilterInput creates the `/`-triggered input field that filters the
// currently-shown rule table.
func (e *Explorer) buildFilterInput() *tview.InputField {
	in := tview.NewInputField().
		SetLabel("/ ").
		SetFieldWidth(0)
	in.SetChangedFunc(func(text string) {
		e.filter = text
		if e.currentChain != nil {
			e.showChain(e.currentChain)
		}
	})
	in.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEscape:
			// Cancel: clear filter and hide input.
			e.filter = ""
			in.SetText("")
			e.hideFilter()
		case tcell.KeyEnter:
			// Keep filter, hide input.
			e.hideFilter()
		}
		if e.currentChain != nil {
			e.showChain(e.currentChain)
		}
	})
	return in
}

// showFilter expands the filter input below the rule table and focuses it.
// No-op unless a chain is currently being shown.
func (e *Explorer) showFilter() {
	if e.currentChain == nil {
		return
	}
	e.rulesFlex.ResizeItem(e.filterInput, 1, 0)
	e.app.SetFocus(e.filterInput)
}

// hideFilter collapses the filter input. The active filter (if any) stays
// applied to the rule table.
func (e *Explorer) hideFilter() {
	e.rulesFlex.ResizeItem(e.filterInput, 0, 0)
	e.app.SetFocus(e.rules)
}

// --- Global search (`:` modal) --------------------------------------------

// buildSearchPage assembles the input + results-list pair that overlays the
// main explorer when `:` is pressed.
func (e *Explorer) buildSearchPage() tview.Primitive {
	e.searchInput = tview.NewInputField().
		SetLabel(": ").
		SetFieldWidth(0)
	e.searchResults = tview.NewList().
		ShowSecondaryText(false)
	e.searchResults.SetBorder(true).
		SetTitle(" Matches ").
		SetTitleAlign(tview.AlignLeft)

	e.searchInput.SetChangedFunc(func(text string) {
		e.refreshSearchResults(text)
	})
	e.searchInput.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEscape:
			e.pages.HidePage("search")
			e.app.SetFocus(e.tree)
		case tcell.KeyEnter:
			// Move focus into the results list so j/k/Enter work there.
			e.app.SetFocus(e.searchResults)
		}
	})
	e.searchResults.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		if ev.Key() == tcell.KeyEscape {
			e.pages.HidePage("search")
			e.app.SetFocus(e.tree)
			return nil
		}
		return ev
	})

	flex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(e.searchInput, 1, 0, true).
		AddItem(e.searchResults, 0, 1, false)
	flex.SetBorder(true).
		SetTitle(" Global search — type to filter, Enter for results, Esc to close ").
		SetTitleAlign(tview.AlignLeft)

	// Centre the modal at ~60% width.
	wrapper := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(nil, 0, 1, false).
		AddItem(
			tview.NewFlex().SetDirection(tview.FlexRow).
				AddItem(nil, 0, 1, false).
				AddItem(flex, 0, 3, true).
				AddItem(nil, 0, 1, false),
			0, 3, true,
		).
		AddItem(nil, 0, 1, false)
	return wrapper
}

// refreshSearchResults re-runs the global search and repopulates the list.
func (e *Explorer) refreshSearchResults(query string) {
	e.searchResults.Clear()
	hits := searchRuleset(e.rs, query, 500)
	if len(hits) == 0 {
		return
	}
	for _, h := range hits {
		label := fmt.Sprintf("%s %s › %s #%d  %s",
			h.chain.Table.Family, h.chain.Table.Name, h.chain.Name,
			h.rule.Handle, truncate(h.rule.NFT, 80))
		e.searchResults.AddItem(label, "", 0, func() {
			e.pages.HidePage("search")
			e.jumpToChain(h.chain)
			e.selectRule(h.rule)
		})
	}
}

// jumpToChain finds the tree node for a chain and selects it, which
// triggers onTreeChange and populates the right pane.
func (e *Explorer) jumpToChain(c *model.Chain) {
	if node, ok := e.nodeByChain[c]; ok {
		e.tree.SetCurrentNode(node)
		e.app.SetFocus(e.tree)
	}
}

// selectRule positions the rules table on the row that corresponds to r,
// taking the active filter into account.
func (e *Explorer) selectRule(r *model.Rule) {
	if e.currentChain == nil {
		return
	}
	display := 1
	for _, rr := range e.currentChain.Rules {
		if !ruleMatches(rr, e.filter) {
			continue
		}
		if rr.Handle == r.Handle {
			e.rules.Select(display, 0)
			e.app.SetFocus(e.rules)
			return
		}
		display++
	}
}
