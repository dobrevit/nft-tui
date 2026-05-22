# Roadmap

Phased so each phase delivers a usable artefact, not just plumbing.

## Phase 1 — Design (this phase)

**Deliverable**: this `docs/` directory. Product brief, architecture,
ASCII screen mockups, data model, keybindings, roadmap. No Go code.

**Exit criteria**: a product reviewer can read the docs and tell us
which screens to cut, which to expand, and whether the safety model
(staged-only edits, dead-man's switch on restore) is right.

## Phase 2 — Read-only explorer

**Deliverable**: a Go binary that connects to the kernel ruleset via
`nft -j list ruleset` and renders:

- Main explorer screen (tree + rule list)
- Sets/maps view
- Live counter refresh (poll only, no netlink yet)
- Search and filter
- `y` to yank `nft` syntax
- Help overlay

No editing, no staging, no commit. Always read-only.

**Why this first.** Reading is 80% of what an admin does. Shipping
read-only first means we get user feedback before designing the write
path, and we never block on the riskier work.

**Done when**: `nft-tui` can be invoked on a router with a 200-rule
ruleset and an admin can find any specific rule (by handle, address,
interface, or comment) in under 10 seconds.

## Phase 3 — Staged edits + commit

**Deliverable**: write mode.

- Rule editor (form + raw mode)
- Add / delete / replace rule, chain, set element
- Staged-changes view
- Dry-run integration (`nft -c -f`)
- Atomic commit (`nft -f`)
- Undo (unstage)
- Snapshot before every commit

**Done when**: an admin can edit a rule, see the diff, dry-run, commit
atomically, and roll back if they don't like the result — without ever
leaving the TUI, and without breaking the existing ruleset on a syntax
error.

## Phase 4 — Live monitor

**Deliverable**: the htop-style live screen.

- Top-N rules by pps / bps / Δpkts
- Sparkline per chain
- Optional netlink monitoring for change events
  (`google/nftables` + `NFT_MSG_NEW*`)

## Phase 5 — Snapshot, restore, and safety

**Deliverable**: the dangerous-actions surface, hardened.

- `:w` / `:r` snapshot file format (just `nft list ruleset` output)
- Restore screen with 60-second dead-man's switch
- Audit log: every commit appends to `/var/log/nft-tui.log` with the
  exact `nft` syntax that was applied and the operator's UID

## Phase 6 — Polish

- Themes (default, high-contrast, mono)
- Mouse support (tview gives us most of this for free)
- Configurable column sets for the rule list
- `--write` flag and runtime `r` toggle finalised
- Packaged: `.deb`, `.rpm`, and a single static binary release on GitHub
- man page

## Out of roadmap (deferred or rejected)

- Multi-host fanout. If you want to manage 50 routers from one terminal,
  use Ansible to push `nft` files; this tool is one host at a time.
- A long-running daemon exporting metrics. Prometheus's
  `node_exporter --collector.nftables` (when it lands) is the right
  layer for that.
- A wizard for novices. Out of scope per the product brief.

## Risks

| Risk                                            | Mitigation                                                                  |
|-------------------------------------------------|-----------------------------------------------------------------------------|
| `nft -j` schema changes between versions        | Pin a minimum `nft` version, parse defensively, fixtures in `testdata/`     |
| Polling `nft -j` is expensive on huge rulesets  | Switch to netlink in Phase 4; cap refresh rate; only refetch on commit/tick |
| Bad commit locks user out over SSH              | Dead-man's switch on restore; refuse to flush `input` chain without confirm |
| Form editor can't represent every nft feature   | Raw mode (F8) is always available; unknown expressions round-trip verbatim  |
| `tview` API churn                               | Vendor a tested version; pin in `go.mod`                                    |
