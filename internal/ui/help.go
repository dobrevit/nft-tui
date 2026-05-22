package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// helpText is the content of the `?` overlay. Kept in sync with
// docs/04-keybindings.md by hand for Phase 2; Phase 3 may auto-generate.
const helpText = `[::b]nft-tui — Phase 2 keybindings[::-]

  [yellow]Navigation[-]
    j / ↓     down
    k / ↑     up
    h / ←     collapse / left pane
    l / →     expand  / right pane
    Enter     open node / toggle expansion
    Tab       cycle pane focus
    g / G     top / bottom

  [yellow]Filter & search[-]
    /         filter the current rule list
    :         global search across the whole ruleset
    Esc       cancel filter / close search

  [yellow]Actions[-]
    y         yank the selected rule's nft syntax to the clipboard (OSC 52)
    R         full reload (use when the status bar shows kernel drift)

  [yellow]Modes[-]
    q         quit
    ?         this help overlay (toggle)

  [gray]Editing, dry-run and commit are not implemented in Phase 2.[-]

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
