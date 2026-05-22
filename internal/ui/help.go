package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// helpText is the content of the `?` overlay. Kept in sync with
// docs/04-keybindings.md by hand for Phase 2; Phase 3 may auto-generate.
const helpText = `[::b]nft-tui — keybindings[::-]

  [yellow]Navigation[-]
    j / ↓     down                Tab       cycle pane focus
    k / ↑     up                  g / G     top / bottom
    h / ←     collapse / left     Enter     open node / toggle
    l / →     expand  / right

  [yellow]Filter & search[-]
    /         filter the current rule list
    :         global search across the whole ruleset
    Esc       cancel filter / close search

  [yellow]Yank & reload[-]
    y         yank the selected rule's nft syntax (OSC 52)
    R         full reload (after kernel drift)
    m         live monitor (top-N by pps/bps/Δpkts)

  [yellow]Edit (requires --write)[-]
    a         add a rule to the current chain
    D         open the staged-changes / diff page

  [yellow]Diff page[-]
    F3        dry-run via nft -c -f
    F2        commit via nft -f (after F3 passes)
    u / U     unstage last / unstage all

  [yellow]Modes[-]
    q         quit
    ?         this help overlay (toggle)

                    Press ? or Esc to close
`

// buildHelpPage wraps a TextView in a centred Flex for the `?` overlay.
func (e *Explorer) buildHelpPage() tview.Primitive {
	view := tview.NewTextView().
		SetDynamicColors(true).
		SetText(helpText)
	view.SetBorder(true).
		SetTitle(" Help ").
		SetTitleAlign(tview.AlignLeft)
	view.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		if ev.Rune() == '?' || ev.Key() == tcell.KeyEscape {
			e.pages.HidePage("help")
			e.app.SetFocus(e.tree)
			return nil
		}
		return ev
	})
	return tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(nil, 0, 1, false).
		AddItem(
			tview.NewFlex().SetDirection(tview.FlexRow).
				AddItem(nil, 0, 1, false).
				AddItem(view, 24, 0, true).
				AddItem(nil, 0, 1, false),
			0, 2, true,
		).
		AddItem(nil, 0, 1, false)
}
