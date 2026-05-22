# Deferred work

Things consciously deferred during Phase 1-3 development, kept here so we
can revisit before declaring v1.0. Each entry says why it was skipped and
roughly what shape the fix would take.

## Phase 3 deferrals

### Form-based rule editor

Phase 3.1 ships raw mode only — a TextArea where the user types nft
syntax directly. The screen mock in [03-screens.md](03-screens.md) shows
a structured form (Interface in / out, Source, Destination, Protocol,
Sport / Dport, ct state, …) with a live nft preview underneath.

**Why deferred.** The form needs (a) parsing existing rules into
structured fields for edit mode, (b) round-tripping unknown clauses
without losing them, (c) UI plumbing for ~20 fields. Raw mode covers
100% of nftables features today; the form is convenience.

**Shape.** Add an `editorFormPage` alongside `editor` page; F8 toggles
between them. In modeEdit, populate fields from the parsed rule and
keep any unrecognised clauses in a "trailing nft" textbox that round-
trips verbatim.

### Explicit confirmation modal before F2 commit

Currently F2 commits if F3 (dry-run) passed. There is no second
"are you sure?" prompt.

**Why deferred.** The dry-run is itself the safety gate. An extra
modal felt like noise during testing. But for restore (Phase 5) we
absolutely need a confirmation, so the pattern will exist; once it
does, gating F2 behind it is a one-liner if you want it.

### Insert-before / insert-after handle in the explorer

`staged.InsertRule` exists in the model and renders correctly
(`insert rule … position H`). The explorer only exposes `a` (append)
and `e` (replace) — no key for "add a rule before / after this one".

**Why deferred.** No good muscle-memory key was obvious. Vim users
would expect `o` / `O`; `mc` users would expect a function key. Wait
until a real workflow demands it.

**Shape.** Add `o` (insert after selected) and `O` (insert before).
Both open the editor in modeAdd but stage an `InsertRule` with the
selected rule's handle as `Position`.

## Phase 4 — skipped intentionally

Phase 4 in the roadmap is the htop-style live monitor with top-N
rules by pps / bps / Δpkts plus a sparkline. Useful but not load-
bearing for the v1.0 commit-then-rollback story.

**Plan.** Resume Phase 4 once Phase 5 ships and we have any user
feedback. The data is already in `model.Rule.Counter`; the in-place
counter merge from Phase 2.2 (`applyRuleset`) gives us a delta per
tick — we just need a screen.

## Earlier deferrals still open

### Tree rebuild on kernel drift

Phase 2.2 detects structural changes from the kernel (new/removed
tables/chains/rules outside our staged ops) and sets `kernelDrift`,
but only forces a manual `R` reload. Could auto-rebuild after a
debounce; tradeoff is losing tree expansion / selection.

### Interval-keyed maps

`internal/nft/setfmt.go renderMapElements` drops `IntervalEnd`
sentinels. Sets with interval keys are common (port ranges); maps
with interval keys are rare. Rendering as `low-high : value` pairs
is the right shape; do it once we see one in the wild.

### Mouse support polish

`tview.Application.EnableMouse(true)` is already on but we never
tested the click-targets carefully (especially modal overlays).

## Code-quality items

These are SonarLint warnings ignored during development; none are
correctness issues.

- **S3776 cognitive complexity** on:
  - `internal/nft/render.go` `RenderRule` (~90)
  - `internal/ui/explorer.go` `applyRuleset` (~17), `showSet` (~33)
  - `internal/ui/diff.go` `renderDiffSummary` (~16)
  - `internal/ui/editor.go` `editorInputCapture` (~10)
  - `internal/ui/search.go` `refreshSearchResults` (~17)

  The hot one is `RenderRule` — the big expr type-switch. Splitting
  each case into a small renderXxx helper is a mechanical refactor.

- **S1192 string literal duplication** — kind constants (`"meta-l4proto"`,
  `"ip-saddr"`, etc.) in `render.go`. Fix is straightforward: define a
  set of `const kindXxx = "…"` strings at the top of the file.

## Not-on-roadmap

These are out-of-scope per [01-product-brief.md](01-product-brief.md)
and stay that way:

- Multi-host fanout (use Ansible).
- Long-running daemon exporting metrics.
- Wizards for novices.
- Iptables-nft / legacy compatibility.
