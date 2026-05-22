# Deferred work

> All items below are resolved for the v1.0 ship — see the ✅ markers
> against each. The page is kept as a record of design decisions and
> as a place to track any new "we'll get to it later" items.

## Phase 3 deferrals (closed)

### Form-based rule editor — ✅ shipped

Phase 3 raw-mode editor is augmented by a structured `tview.Form` in
`internal/ui/editor_form.go`. The form covers the common-case fields
(iifname / oifname / saddr / daddr / proto+sport+dport / ct state /
counter / log+prefix / verdict / comment) and live-updates the same
TextArea the raw view edits — single source of truth for stage and
preview. **F8** toggles between form and raw views; modeAdd /
modeInsert default to form, modeEdit defaults to raw (no nft parser
yet, so structured round-trip would be lossy).

Open follow-up: parsing existing rules into the form on modeEdit
without losing unrecognised clauses. Would need a small nft parser
or a "trailing-nft" passthrough field.

### Explicit confirmation modal before F2 commit — ✅ shipped

F2 now opens a `tview.Modal` with the change count and Apply / Cancel
buttons. The dry-run gate is still required first.

### Insert-before / insert-after handle — ✅ shipped

`o` (insert AFTER the selected rule, → `add rule … position H`) and
`O` (insert BEFORE, → `insert rule … position H`). `staged.InsertRule`
gained an `After bool` so both nft verbs round-trip.

## Earlier deferrals (closed)

### Tree rebuild on kernel drift — ✅ shipped

`applyRuleset` now auto-rebuilds the tree on structural diffs (paired
with Phase 4.3 monitor events that fire on external mutations within
250 ms). `rebuildPreservingSelection` preserves the user's
currently-viewed chain across the rebuild by `(family, table, name)`
tuple; if the chain was deleted externally, the right pane swaps to
an explanatory info view so the user isn't silently relocated.

### Interval-keyed maps — ✅ shipped

`renderMapElements` pairs `IntervalEnd` sentinels with the preceding
start element to produce `low-high : value`. Tested for plain maps,
interval maps, missing values, and pathological orphan inputs.

### Mouse support polish — ✅ audited, defaults documented

Audited. `EnableMouse(true)` is on; tview's primitive defaults cover
click-to-select on trees / table rows / list items / modal buttons,
and scroll-wheel on text views and tables. Help overlay documents
the primary mouse interactions.

Open follow-up if it ever bites: modal-overlay leakage scenarios —
clicks on the padding around a centred overlay falling through to the
underlying main page — weren't exercised because reproducing them
needs an interactive terminal. Fix would be a `Box`-with-mouse-capture
wrapper around each centred overlay.

## Code-quality items (closed)

### S1192 string-literal duplication — ✅ fixed

11 kind discriminators (`kindMetaIIFName`, `kindCTState`, `kindIPSAddr`,
…) are now package-private constants in `internal/nft/render.go`. Any
future kind-comparison typo is a compile error instead of a silent
fall-through.

### S3776 cognitive complexity on RenderRule — ✅ refactored

`RenderRule` is now a 12-line dispatch loop over a `ruleRenderer`
struct; each expression type has its own small `handle*` method and
the Cmp case is further split into `cmpCTStateMask` / `cmpL4Proto` /
`cmpNFProto` / `cmpGeneric`. The biggest function the renderer ships
is `handleExpr` (the type switch itself), which is just dispatch.

### S3776 on the remaining functions — ⚪ open polish

Lower-priority complexity warnings remain on:

- `internal/ui/explorer.go` `applyRuleset`, `showSet`, `handleKey`,
  `rebuildPreservingSelection`
- `internal/ui/diff.go` `renderDiffSummary`
- `internal/ui/editor.go` `editorInputCapture`
- `internal/ui/search.go` `refreshSearchResults`
- `internal/nft/commit_integration_test.go` integration test setup
- `cmd/nft-tui/main.go` `main`

None affect correctness; the linter flags ≥16. The patterns are all
sequential setup-then-dispatch — splitting buys readability and not
much else. Tackle when a function actually becomes hard to read.

## Open v1.1+ ideas

These aren't blockers for v1.0 — they're things that would be nice to
add if we ever get user feedback asking for them.

### nft text → form fields parser

Lets modeEdit open the form prefilled from an existing rule.
Trailing-nft passthrough field for unrecognised clauses.

### Theme-driven dynamic-colour tags

Phase 6.0's `Theme` struct exposes semantic colours (Accent / Good /
Bad / Warning / Muted / Header) but the codebase still embeds raw
`[green]`/`[yellow]`/`[red]` tags. Plumbing the semantic colours
through a render helper would let high-contrast / mono themes actually
remap accents instead of relying only on tview.Styles changes.

### Per-host config file

`~/.config/nft-tui/config.toml` for defaults: theme, columns,
refresh interval, audit dir override. CLI flags still override.

## Not-on-roadmap (won't fix)

These are out-of-scope per [01-product-brief.md](01-product-brief.md)
and stay that way:

- Multi-host fanout (use Ansible).
- Long-running daemon exporting metrics.
- Wizards for novices.
- iptables-nft / legacy compatibility.
