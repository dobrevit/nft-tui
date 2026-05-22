package ui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/dobrevit/nft-tui/internal/model"
	"github.com/dobrevit/nft-tui/internal/staged"
)

// editorMode selects between add-rule, edit-existing-rule, and
// insert-relative-to-rule semantics.
type editorMode int

const (
	modeAdd editorMode = iota
	modeEdit
	// modeInsert stages a staged.InsertRule against editorTarget.position
	// (anchor handle). editorTarget.after selects before vs after.
	modeInsert
)

// editorTarget identifies what the editor will produce a StagedOp against.
// handle  — non-zero in modeEdit (the rule to replace)
// position/after — set in modeInsert (the anchor rule + relative side)
type editorTarget struct {
	family   model.Family
	table    string
	chain    string
	handle   uint64
	position uint64
	after    bool
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

	// Construct the preview FIRST. buildEditorForm initialises its
	// DropDown widgets via SetCurrentOption(0), which synchronously
	// fires the onSelect callback → push() → editorBody.SetText →
	// refreshEditorPreview. If editorPreview isn't built yet that
	// chain dereferences a nil receiver and panics. (Ditto for the
	// raw TextArea writing into an unset preview.)
	e.editorPreview = tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true)
	e.editorPreview.SetBorder(true).
		SetTitle(" Preview (will be staged) ").
		SetTitleAlign(tview.AlignLeft)

	e.editorBody = tview.NewTextArea().
		SetPlaceholder(`e.g. tcp dport 22 counter accept`)
	e.editorBody.SetBorder(true).
		SetTitle(" Rule body — raw nft syntax — F8 to switch to form ").
		SetTitleAlign(tview.AlignLeft)
	e.editorBody.SetChangedFunc(e.refreshEditorPreview)

	e.editorForm = e.buildEditorForm()

	e.editorComment = tview.NewInputField().
		SetLabel("Comment: ").
		SetFieldWidth(0)
	e.editorComment.SetChangedFunc(func(_ string) { e.refreshEditorPreview() })

	footer := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft).
		SetText("[yellow]F5[-] stage   [yellow]F6[-] stage+diff   [yellow]F8[-] form/raw   [yellow]Esc[-] cancel")

	// editorViews holds form and raw side-by-side at the same depth;
	// only one is visible at a time. Initial state: form on, raw off.
	e.editorViews = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(e.editorForm, 0, 3, true).
		AddItem(e.editorBody, 0, 0, false)

	inner := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(e.editorTitle, 1, 0, false).
		AddItem(e.editorViews, 0, 3, true).
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
	case tcell.KeyF8:
		e.toggleEditorView()
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
// Requires writeMode; no-op otherwise. Defaults to the form view —
// new rules are usually simple enough that the structured form is
// faster than typing nft by hand.
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
	e.resetForm()
	e.showFormView()
	e.refreshEditorPreview()
	e.pages.ShowPage("editor")
	e.app.SetFocus(e.editorForm)
}

// openEditorReplace opens the editor in modeEdit for the supplied rule.
// The body is prefilled with the rule's rendered NFT text. Edits
// default to the raw view because parsing existing nft text into the
// form's structured fields is more complex than this phase ships;
// the operator can hit F8 once they've cleared the body to switch.
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
	e.resetForm()
	e.editorBody.SetText(body, true)
	e.editorComment.SetText(comment)
	e.showRawView()
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
	case modeInsert:
		return &staged.InsertRule{
			Family:   e.editorTarget.family,
			Table:    e.editorTarget.table,
			Chain:    e.editorTarget.chain,
			Position: e.editorTarget.position,
			After:    e.editorTarget.after,
			Body:     body,
			Comment:  comment,
		}
	}
	return nil
}

// openEditorInsert opens the editor in modeInsert anchored on r. The
// `after` flag selects between `insert rule … position H` (before)
// and `add rule … position H` (after).
func (e *Explorer) openEditorInsert(r *model.Rule, after bool) {
	if !e.writeMode {
		e.setStatus("[yellow]read-only mode — start with --write to insert[-]")
		return
	}
	c := r.Chain
	e.editorMode = modeInsert
	e.editorTarget = editorTarget{
		family:   c.Table.Family,
		table:    c.Table.Name,
		chain:    c.Name,
		position: r.Handle,
		after:    after,
	}
	rel := "before"
	if after {
		rel = "after"
	}
	e.editorTitle.SetText(fmt.Sprintf(
		"[::b]Insert rule %s handle %d[::-]   %s %s %s",
		rel, r.Handle, c.Table.Family, c.Table.Name, c.Name))
	e.editorBody.SetText("", true)
	e.editorComment.SetText("")
	e.resetForm()
	e.showFormView()
	e.refreshEditorPreview()
	e.pages.ShowPage("editor")
	e.app.SetFocus(e.editorForm)
}

// showFormView and showRawView force the editor into one of the two
// view modes without going through the F8 toggle (used by the open*
// helpers to set the default on each open).
func (e *Explorer) showFormView() {
	e.editorView = viewForm
	e.editorViews.ResizeItem(e.editorBody, 0, 0)
	e.editorViews.ResizeItem(e.editorForm, 0, 3)
}

func (e *Explorer) showRawView() {
	e.editorView = viewRaw
	e.editorViews.ResizeItem(e.editorForm, 0, 0)
	e.editorViews.ResizeItem(e.editorBody, 0, 3)
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
