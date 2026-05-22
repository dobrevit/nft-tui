package ui

import (
	"fmt"
	"strings"

	"github.com/rivo/tview"
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

	// Tracked form values. Captured once and mutated by the field
	// callbacks below.
	e.formFields = &formFields{ctStates: make([]bool, len(ctStateLabels))}
	ff := e.formFields

	push := func() {
		body := ff.regenerateBody()
		// Direct push — bypassing SetChangedFunc avoids recursion if
		// SetText would itself trigger the body's own change callback.
		e.editorBody.SetText(body, true)
		e.refreshEditorPreview()
	}

	f.AddInputField("iifname", "", 16, nil, func(s string) { ff.iifname = s; push() })
	f.AddInputField("oifname", "", 16, nil, func(s string) { ff.oifname = s; push() })
	f.AddInputField("ip saddr", "", 24, nil, func(s string) { ff.saddr = s; push() })
	f.AddInputField("ip daddr", "", 24, nil, func(s string) { ff.daddr = s; push() })

	protos := []string{"any", "tcp", "udp", "icmp", "icmpv6"}
	f.AddDropDown("L4 proto", protos, 0, func(opt string, _ int) {
		ff.proto = opt
		push()
	})

	f.AddInputField("sport", "", 10, nil, func(s string) { ff.sport = s; push() })
	f.AddInputField("dport", "", 10, nil, func(s string) { ff.dport = s; push() })

	for i, label := range ctStateLabels {
		idx := i
		f.AddCheckbox("ct "+label, false, func(checked bool) {
			ff.ctStates[idx] = checked
			push()
		})
	}

	f.AddCheckbox("counter", false, func(c bool) { ff.counter = c; push() })
	f.AddCheckbox("log", false, func(c bool) { ff.log = c; push() })
	f.AddInputField("log prefix", "", 24, nil, func(s string) { ff.logPref = s; push() })

	verdicts := []string{"", "accept", "drop", "reject", "return"}
	f.AddDropDown("verdict", verdicts, 0, func(opt string, _ int) {
		ff.verdict = opt
		push()
	})

	return f
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
