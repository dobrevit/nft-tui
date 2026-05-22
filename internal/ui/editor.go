package ui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/dobrevit/nft-tui/internal/model"
	"github.com/dobrevit/nft-tui/internal/staged"
)

// editorMode selects between add-rule and edit-existing-rule semantics.
type editorMode int

const (
	modeAdd editorMode = iota
	modeEdit
)

// editorTarget identifies what the editor will produce a StagedOp against.
// Handle is zero in modeAdd.
type editorTarget struct {
	family model.Family
	table  string
	chain  string
	handle uint64
}

// buildEditorPage assembles the modal-style page that hosts the rule
// editor. Phase 3.1 ships raw mode only: the user types nft syntax
// directly into a TextArea and sees the full `add rule …` / `replace
// rule …` statement live in the preview pane.
//
// F5 stages, Esc cancels. F6 (stage + jump to diff) is wired in 3.2.
func (e *Explorer) buildEditorPage() tview.Primitive {
	e.editorTitle = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)

	e.editorBody = tview.NewTextArea().
		SetPlaceholder(`e.g. tcp dport 22 counter accept`)
	e.editorBody.SetBorder(true).
		SetTitle(" Rule body — nft syntax ").
		SetTitleAlign(tview.AlignLeft)
	e.editorBody.SetChangedFunc(e.refreshEditorPreview)

	e.editorComment = tview.NewInputField().
		SetLabel("Comment: ").
		SetFieldWidth(0)
	e.editorComment.SetChangedFunc(func(_ string) { e.refreshEditorPreview() })

	e.editorPreview = tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true)
	e.editorPreview.SetBorder(true).
		SetTitle(" Preview (will be staged) ").
		SetTitleAlign(tview.AlignLeft)

	footer := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft).
		SetText("[yellow]F5[-] stage   [yellow]Esc[-] cancel   [gray](F6 stage+diff coming in 3.2)[-]")

	inner := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(e.editorTitle, 1, 0, false).
		AddItem(e.editorBody, 0, 3, true).
		AddItem(e.editorComment, 1, 0, false).
		AddItem(e.editorPreview, 0, 2, false).
		AddItem(footer, 1, 0, false)
	inner.SetBorder(true).
		SetTitle(" Edit rule ").
		SetTitleAlign(tview.AlignLeft)
	inner.SetInputCapture(e.editorInputCapture)

	// Centre at ~75% width / 80% height.
	return tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(nil, 0, 1, false).
		AddItem(
			tview.NewFlex().SetDirection(tview.FlexRow).
				AddItem(nil, 0, 1, false).
				AddItem(inner, 0, 8, true).
				AddItem(nil, 0, 1, false),
			0, 6, true,
		).
		AddItem(nil, 0, 1, false)
}

func (e *Explorer) editorInputCapture(ev *tcell.EventKey) *tcell.EventKey {
	switch ev.Key() {
	case tcell.KeyEsc:
		e.closeEditor()
		return nil
	case tcell.KeyF5:
		e.stageFromEditor(false)
		return nil
	case tcell.KeyF6:
		// Stage and jump to the diff page so the operator can review
		// (and Phase 3.3 commit) without an extra keystroke.
		e.stageFromEditor(true)
		e.openDiff()
		return nil
	case tcell.KeyTab:
		// Cycle focus among the three editable widgets.
		switch e.app.GetFocus() {
		case e.editorBody:
			e.app.SetFocus(e.editorComment)
		case e.editorComment:
			e.app.SetFocus(e.editorBody)
		}
		return nil
	}
	return ev
}

// openEditorAdd opens the editor in modeAdd targeting the supplied chain.
// Requires writeMode; no-op otherwise.
func (e *Explorer) openEditorAdd(c *model.Chain) {
	if !e.writeMode {
		e.setStatus("[yellow]read-only mode — start with --write to edit[-]")
		return
	}
	e.editorMode = modeAdd
	e.editorTarget = editorTarget{
		family: c.Table.Family,
		table:  c.Table.Name,
		chain:  c.Name,
	}
	e.editorTitle.SetText(fmt.Sprintf(
		"[::b]Add rule[::-]   %s %s %s",
		c.Table.Family, c.Table.Name, c.Name))
	e.editorBody.SetText("", true)
	e.editorComment.SetText("")
	e.refreshEditorPreview()
	e.pages.ShowPage("editor")
	e.app.SetFocus(e.editorBody)
}

// openEditorReplace opens the editor in modeEdit for the supplied rule.
// The body is prefilled with the rule's rendered NFT text.
func (e *Explorer) openEditorReplace(r *model.Rule) {
	if !e.writeMode {
		e.setStatus("[yellow]read-only mode — start with --write to edit[-]")
		return
	}
	c := r.Chain
	e.editorMode = modeEdit
	e.editorTarget = editorTarget{
		family: c.Table.Family,
		table:  c.Table.Name,
		chain:  c.Name,
		handle: r.Handle,
	}
	e.editorTitle.SetText(fmt.Sprintf(
		"[::b]Edit rule[::-]   %s %s %s   handle %d",
		c.Table.Family, c.Table.Name, c.Name, r.Handle))
	// Strip any trailing `comment "X"` so the body field is the body only;
	// the comment goes into its own field.
	body, comment := splitCommentFromNFT(r.NFT)
	if r.Comment != "" {
		comment = r.Comment
	}
	e.editorBody.SetText(body, true)
	e.editorComment.SetText(comment)
	e.refreshEditorPreview()
	e.pages.ShowPage("editor")
	e.app.SetFocus(e.editorBody)
}

// closeEditor hides the editor page and returns focus to the tree.
func (e *Explorer) closeEditor() {
	e.pages.HidePage("editor")
	e.app.SetFocus(e.tree)
}

// refreshEditorPreview rebuilds the preview pane based on the current
// text-area + comment contents.
func (e *Explorer) refreshEditorPreview() {
	op := e.currentEditorOp()
	if op == nil {
		e.editorPreview.SetText("[gray](empty — type a rule body to preview)[-]")
		return
	}
	e.editorPreview.SetText(op.NFT())
}

// currentEditorOp builds the staged.Op the editor would produce right
// now. Returns nil if the body is empty.
func (e *Explorer) currentEditorOp() staged.Op {
	body := strings.TrimSpace(e.editorBody.GetText())
	if body == "" {
		return nil
	}
	comment := strings.TrimSpace(e.editorComment.GetText())
	switch e.editorMode {
	case modeAdd:
		return &staged.AddRule{
			Family:  e.editorTarget.family,
			Table:   e.editorTarget.table,
			Chain:   e.editorTarget.chain,
			Body:    body,
			Comment: comment,
		}
	case modeEdit:
		return &staged.ReplaceRule{
			Family:  e.editorTarget.family,
			Table:   e.editorTarget.table,
			Chain:   e.editorTarget.chain,
			Handle:  e.editorTarget.handle,
			Body:    body,
			Comment: comment,
		}
	}
	return nil
}

// stageFromEditor appends the current editor contents to the staged list
// and closes the editor. The bool is reserved for Phase 3.2's "stage and
// jump to diff" affordance.
func (e *Explorer) stageFromEditor(_ bool) {
	op := e.currentEditorOp()
	if op == nil {
		e.setStatus("[yellow]editor: rule body is empty — nothing to stage[-]")
		return
	}
	e.staged.Append(op)
	e.closeEditor()
	e.refreshStatusBar(e.rs.FetchedAt)
	e.setStatus(fmt.Sprintf(
		"staged %s (total: %d) — Phase 3.3 will let you commit",
		op.Describe(), e.staged.Len()))
}

// splitCommentFromNFT finds a trailing ` comment "X"` token in raw nft
// text and returns the body without it plus the unquoted comment. If no
// trailing comment is present, returns the whole text and "".
func splitCommentFromNFT(s string) (body, comment string) {
	i := strings.LastIndex(s, ` comment "`)
	if i < 0 {
		return s, ""
	}
	rest := s[i+len(` comment "`):]
	end := strings.LastIndex(rest, `"`)
	if end < 0 {
		return s, ""
	}
	return s[:i], rest[:end]
}
