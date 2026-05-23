package ui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/dobrevit/nft-tui/internal/model"
)

// editorViewMode selects between the structured form and the raw-nft
// TextArea inside the editor page. Toggled with F8.
type editorViewMode int

const (
	viewForm editorViewMode = iota
	viewRaw
)

// formFields holds the bound values of every form widget. The bound
// closures push their values into the right field; regenerateBody
// turns the struct into an nft body string.
//
// Phase 8 form covers the most common rule shapes (interface + source +
// dest + proto/dport + verdict + counter + log + comment). Anything
// beyond that pushes the operator to F8 raw mode — which is always
// available and round-trips 100% of nftables.
type formFields struct {
	iifname  string
	oifname  string
	saddr    string
	daddr    string
	proto    string // "any" | "tcp" | "udp" | "icmp" | "icmpv6"
	sport    string
	dport    string
	counter  bool
	log      bool
	logPref  string
	verdict  string // "accept" | "drop" | "reject" | "return"
	ctStates []bool // [established, related, new, invalid]
}

var ctStateLabels = []string{"established", "related", "new", "invalid"}

// formFieldsFromRule maps an already-decoded model.Rule back into a
// formFields struct. Used when openEditorReplace decides the form
// view is safe to open (rule has no unknown expressions).
//
// Fields the renderer doesn't decode (log + log prefix) are left at
// their zero value — the form view will not pre-populate them, but
// the operator can re-tick them and the regenerated body will still
// include other parts. Log preservation would need the renderer to
// extract Log.Data, which it currently only emits as text.
func formFieldsFromRule(r *model.Rule) *formFields {
	ff := &formFields{ctStates: make([]bool, len(ctStateLabels))}
	ff.iifname = r.IIfName
	ff.oifname = r.OIfName
	ff.saddr = r.SAddr
	ff.daddr = r.DAddr
	ff.sport = r.SPort
	ff.dport = r.DPort
	ff.counter = r.Counter.Present
	ff.verdict = r.Verdict

	// Proto: the form dropdown has "any" as the zero option; map an
	// empty Proto onto that so it round-trips cleanly.
	if r.Proto == "" {
		ff.proto = "any"
	} else {
		ff.proto = r.Proto
	}

	// CT state: model.Rule.CTState renders as either a single name
	// ("established") or a comma-joined list inside braces
	// ("{ established, related }"). Strip the braces if present
	// and tick each known name.
	cts := strings.Trim(strings.TrimSpace(r.CTState), "{} ")
	if cts != "" {
		for _, name := range strings.Split(cts, ",") {
			name = strings.TrimSpace(name)
			for i, known := range ctStateLabels {
				if known == name {
					ff.ctStates[i] = true
				}
			}
		}
	}
	return ff
}

// regenerateBody builds an nft rule body from the current form fields.
// Order matches nft's canonical layout (matches first, then statements,
// then verdict). The empty-field check skips each clause cleanly so a
// half-filled form still produces valid syntax.
func (f *formFields) regenerateBody() string {
	var parts []string

	if f.iifname != "" {
		parts = append(parts, fmt.Sprintf(`iifname %q`, f.iifname))
	}
	if f.oifname != "" {
		parts = append(parts, fmt.Sprintf(`oifname %q`, f.oifname))
	}
	if f.saddr != "" {
		parts = append(parts, "ip saddr "+f.saddr)
	}
	if f.daddr != "" {
		parts = append(parts, "ip daddr "+f.daddr)
	}
	if states := f.ctStateClause(); states != "" {
		parts = append(parts, states)
	}
	if f.proto != "" && f.proto != "any" {
		if f.sport != "" {
			parts = append(parts, fmt.Sprintf("%s sport %s", f.proto, f.sport))
		}
		if f.dport != "" {
			parts = append(parts, fmt.Sprintf("%s dport %s", f.proto, f.dport))
		}
	}
	if f.log {
		s := "log"
		if f.logPref != "" {
			s = fmt.Sprintf(`log prefix %q`, f.logPref)
		}
		parts = append(parts, s)
	}
	if f.counter {
		parts = append(parts, "counter")
	}
	if f.verdict != "" {
		parts = append(parts, f.verdict)
	}
	return strings.Join(parts, " ")
}

// ctStateClause renders the `ct state { … }` match from the bound
// checkboxes. Returns "" if nothing is selected.
func (f *formFields) ctStateClause() string {
	var on []string
	for i, b := range f.ctStates {
		if b {
			on = append(on, ctStateLabels[i])
		}
	}
	switch len(on) {
	case 0:
		return ""
	case 1:
		return "ct state " + on[0]
	default:
		return "ct state { " + strings.Join(on, ", ") + " }"
	}
}

// buildEditorForm constructs the tview.Form whose changes feed
// e.editorBody (the same TextArea the raw view edits). One source of
// truth means the existing preview / stage path needs no changes.
func (e *Explorer) buildEditorForm() *tview.Form {
	f := tview.NewForm()
	f.SetBorder(true).
		SetTitle(" Form — F8 to switch to raw nft ").
		SetTitleAlign(tview.AlignLeft)

	// Tracked form values. If the caller pre-populated e.formFields
	// (modeEdit prefilling from an existing rule), use it as the
	// initial state so the widgets below render with the right
	// values; otherwise start from zero.
	if e.formFields == nil {
		e.formFields = &formFields{ctStates: make([]bool, len(ctStateLabels))}
	}
	ff := e.formFields

	push := func() {
		body := ff.regenerateBody()
		// Direct push — bypassing SetChangedFunc avoids recursion if
		// SetText would itself trigger the body's own change callback.
		e.editorBody.SetText(body, true)
		e.refreshEditorPreview()
	}

	// Each widget reads its initial value from ff so an external
	// caller (modeEdit prefill via formFieldsFromRule) sees their
	// state on screen. A fresh modeAdd open passes a zero-value ff
	// and so renders blank fields.
	f.AddInputField("iifname", ff.iifname, 16, nil, func(s string) { ff.iifname = s; push() })
	f.AddInputField("oifname", ff.oifname, 16, nil, func(s string) { ff.oifname = s; push() })
	f.AddInputField("ip saddr", ff.saddr, 24, nil, func(s string) { ff.saddr = s; push() })
	f.AddInputField("ip daddr", ff.daddr, 24, nil, func(s string) { ff.daddr = s; push() })

	protos := []string{"any", "tcp", "udp", "icmp", "icmpv6"}
	f.AddDropDown("L4 proto", protos, indexOf(protos, ff.proto, 0), func(opt string, _ int) {
		ff.proto = opt
		push()
	})

	f.AddInputField("sport", ff.sport, 10, nil, func(s string) { ff.sport = s; push() })
	f.AddInputField("dport", ff.dport, 10, nil, func(s string) { ff.dport = s; push() })

	for i, label := range ctStateLabels {
		idx := i
		initial := ff.ctStates[i]
		f.AddCheckbox("ct "+label, initial, func(checked bool) {
			ff.ctStates[idx] = checked
			push()
		})
	}

	f.AddCheckbox("counter", ff.counter, func(c bool) { ff.counter = c; push() })
	f.AddCheckbox("log", ff.log, func(c bool) { ff.log = c; push() })
	f.AddInputField("log prefix", ff.logPref, 24, nil, func(s string) { ff.logPref = s; push() })

	verdicts := []string{"", "accept", "drop", "reject", "return"}
	f.AddDropDown("verdict", verdicts, indexOf(verdicts, ff.verdict, 0), func(opt string, _ int) {
		ff.verdict = opt
		push()
	})

	// Scroll hints: when the focused field is anywhere but the first
	// or last, draw ▲/▼ glyphs in the form's border so the operator
	// sees there's more content above/below than the visible window
	// is showing. Useful on small terminals where the form's 14 fields
	// don't all fit.
	f.SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
		idx, _ := f.GetFocusedItemIndex()
		last := f.GetFormItemCount() - 1
		drawBoxScrollHints(screen, x, y, width, height, idx > 0, idx >= 0 && idx < last)
		return f.GetInnerRect()
	})

	return f
}

// drawBoxScrollHints overlays small ▲/▼ glyphs on the right side of a
// bordered primitive's top/bottom border row to signal that there is
// more content above or below the visible area. Called from a Box's
// drawFunc — runs after the border is rendered, so we just replace the
// horizontal-border rune at one cell.
func drawBoxScrollHints(screen tcell.Screen, x, y, width, height int, above, below bool) {
	if width < 4 || height < 2 {
		return
	}
	col := x + width - 3
	style := tcell.StyleDefault.Foreground(tcell.ColorYellow)
	if above {
		screen.SetContent(col, y, '▲', nil, style)
	}
	if below {
		screen.SetContent(col, y+height-1, '▼', nil, style)
	}
}

// attachTextViewScrollHints wires drawBoxScrollHints onto a TextView's
// border using its real scroll offset and wrapped line count. Use for
// bordered text panes that may exceed the visible area (help, preview,
// diff summary/script).
func attachTextViewScrollHints(view *tview.TextView) {
	view.SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
		row, _ := view.GetScrollOffset()
		total := view.GetWrappedLineCount()
		inner := height - 2
		drawBoxScrollHints(screen, x, y, width, height, row > 0, row+inner < total)
		return view.GetInnerRect()
	})
}

// attachTableScrollHints wires drawBoxScrollHints onto a Table's border
// based on its scroll offset vs row count. Header rows fixed via
// SetFixed are not subtracted — the offset is reported in absolute row
// coordinates and the slight over-estimate (▼ may flicker off one row
// earlier) is fine for an indicator.
func attachTableScrollHints(table *tview.Table) {
	table.SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
		row, _ := table.GetOffset()
		total := table.GetRowCount()
		inner := height - 2
		drawBoxScrollHints(screen, x, y, width, height, row > 0, row+inner < total)
		return table.GetInnerRect()
	})
}

// indexOf returns the position of want in opts, or fallback if not
// present. Used to map prefilled field values back to tview.Form
// dropdown indices.
func indexOf(opts []string, want string, fallback int) int {
	for i, o := range opts {
		if o == want {
			return i
		}
	}
	return fallback
}

// resetForm clears all form fields back to their zero values, called
// when the editor reopens for a fresh rule.
func (e *Explorer) resetForm() {
	if e.editorForm == nil {
		return
	}
	e.formFields = &formFields{ctStates: make([]bool, len(ctStateLabels))}
	// Rebuild the form to reset every widget — tview.Form has no
	// "reset all" API; clearing and re-adding is the supported path.
	e.editorForm.Clear(true)
	*e.editorForm = *e.buildEditorForm()
}

// toggleEditorView swaps between viewForm and viewRaw on F8.
// The editorBody TextArea is the single source of truth; in form mode
// it is read-only-from-the-user (the form writes it) and hidden;
// in raw mode it is shown and the form is hidden.
func (e *Explorer) toggleEditorView() {
	if e.editorView == viewForm {
		e.editorView = viewRaw
		// Hide the form, show the TextArea — keep current body content.
		e.editorViews.ResizeItem(e.editorForm, 0, 0)
		e.editorViews.ResizeItem(e.editorBody, 0, 3)
		e.app.SetFocus(e.editorBody)
	} else {
		e.editorView = viewForm
		e.editorViews.ResizeItem(e.editorBody, 0, 0)
		e.editorViews.ResizeItem(e.editorForm, 0, 3)
		e.app.SetFocus(e.editorForm)
	}
}
