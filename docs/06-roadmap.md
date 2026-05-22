# Roadmap

Phased so each phase delivers a usable artefact, not just plumbing.

> **v1.0 ready** — all phases below are shipped. See the foot of this file
> for the [v1.0 release summary](#v10---ready).

## Phase 1 — Design ✅ shipped

**Deliverable**: this `docs/` directory. Product brief, architecture,
ASCII screen mockups, data model, keybindings, roadmap.

**Exit criteria** met: design covers screens, safety model
(staged-only edits, dead-man's switch on restore), and a clear
implementation plan for Phases 2–6.

## Phase 2 — Read-only explorer ✅ shipped

**Note vs. the original plan.** Switched from `nft -j list ruleset`
(text-based, shell-out) to `google/nftables` netlink during early
implementation. The renderer in `internal/nft/render.go` walks the
binary expression AST directly. See
[02-architecture.md](02-architecture.md) for the rationale.

Delivered slices:

- 2.0 — netlink read path + tview explorer
- 2.1 — renderer correctness (anonymous-set inlining, nfproto
  guard elision, rule comments, set elements)
- 2.2 — live counter refresh on a ticker, in-place merge by
  `(family, table, chain, handle)`
- 2.3 — `/` filter and `:` global search
- 2.4 — `y` yank (OSC 52), `?` help overlay
- 2.5 — pre-computed `SearchKey` for sub-millisecond filtering on
  50k-rule rulesets (4–5× faster, ~100,000× fewer allocs)
- 2.6 — maps as first-class (verdict maps as `vmap`, data maps as
  `map`, split sets/maps tree groups)

**Exit criterion met**: an admin can find a specific rule on a
200-rule (or 50k-rule) ruleset in well under 10 seconds via
`:` global search.

## Phase 3 — Staged edits + commit ✅ shipped

- 3.0 — write-path adapter (`internal/staged` + `internal/nft/commit.go`)
- 3.1 — rule editor (raw nft TextArea + live preview)
- 3.2 — staged-changes diff page with before/after for replaces
- 3.3 — F2 commit, gated on a passing F3 dry-run, audit-archived
- 3.4 — `a` / `e` / `d` keys on the rule list

**Exit criterion met**: edit → diff → dry-run → atomic commit, with
rollback via the audit archive, never leaving the TUI. Verified by
`TestIntegration_DryRunAndCommit` (add → replace → delete, all
round-tripped through the kernel).

## Phase 4 — Live monitor ✅ shipped

- 4.0 — per-rule pps/bps deltas via in-place counter merge
- 4.1 — `m` opens the live monitor (top-N by pps/bps/Δpkts),
  `s` cycles sort, `p` pauses
- 4.2 — per-rule sparklines (60-sample ring buffer, Unicode block
  rendering)
- 4.3 — `--monitor` netlink subscription for instant refresh on
  external changes (`NFT_MSG_NEW*` / `NFT_MSG_DEL*`), 250 ms
  debounce coalescing — a 50-rule transaction collapses to 1 event

## Phase 5 — Snapshot, restore, and safety ✅ shipped

- 5.0 — `Committer.Snapshot` / `Committer.Restore`, dry-run-first
- 5.1 — `:` command line: `:w <path>` snapshots, `:r <path>` restores
- 5.2 — 60-second dead-man's switch on `:r`: confirm-to-apply →
  countdown overlay with shrinking colour-coded bar → Y to keep,
  Esc to roll back, timeout → auto-rollback
- 5.3 — append-only `audit.log` with timestamp / UID / user /
  action / nft script

The rollback snapshot is captured BEFORE the restore is applied,
so a network lockout from a bad ruleset still recovers
automatically.

## Phase 6 — Polish ✅ shipped

- 6.0 — three themes (`default`, `high-contrast`, `mono`)
- 6.1 — groff `man/nft-tui.1`
- 6.2 — `.goreleaser.yaml` (linux/amd64 + arm64, .deb, .rpm,
  tar.gz, checksums) + `Makefile` + `-version` flag
- 6.3 — four column presets (`default`, `minimal`, `debug`, `wide`),
  `--columns` flag, `c` cycles at runtime

Mouse support and the deferred work list are documented in
[07-deferred.md](07-deferred.md).

## v1.0 — ready

**What ships:**

- **Read path**: netlink via `google/nftables`. Polling refresh
  (default 2 s) plus optional `NFT_MSG_NEW*` subscription that
  refreshes within 250 ms of any external mutation.
- **Write path**: staged ops → `nft -c` dry-run → `nft -f` atomic
  commit. Every commit archived to `$XDG_STATE_HOME/nft-tui/` as a
  paste-ready file + a line in the rolling `audit.log`.
- **Restore safety**: `:w` snapshot, `:r` restore with a 60-second
  dead-man's switch that auto-rolls-back if not confirmed.
- **UI**: tview-based, mouse-enabled, themed. Tree explorer + rule
  table (configurable columns), live monitor with per-rule
  sparklines, `/` filter, `:` global search, structured form +
  raw nft editor, diff page.
- **CLI**: `-write` `-refresh` `-monitor` `-audit-dir` `-theme`
  `-columns` `-dump` `-version`.
- **Distribution**: single static binary (`CGO_ENABLED=0`),
  goreleaser-built .deb / .rpm / tar.gz, man page.
- **Tests**: 47 unit tests, 4 integration tests behind a build tag
  (`unshare -rn go test -tags=integration ./internal/nft/`).

**Tagged in git**: `v0.1.0` (planned).

**Known follow-up work**: see [07-deferred.md](07-deferred.md). None
of the open items are blockers — they're polish, future-proofing,
or features deliberately deferred (e.g., theme-driven dynamic
colour tags, multi-host fanout — that one stays out).

## Out of roadmap (deferred or rejected)

- Multi-host fanout. If you want to manage 50 routers from one terminal,
  use Ansible to push `nft` files; this tool is one host at a time.
- A long-running daemon exporting metrics. Prometheus's
  `node_exporter --collector.nftables` (when it lands) is the right
  layer for that.
- A wizard for novices. Out of scope per the product brief.

## Risks (retrospective)

The Phase 1 risk register, with notes on how each one played out:

| Risk                                            | Outcome                                                                  |
|-------------------------------------------------|--------------------------------------------------------------------------|
| `nft -j` schema changes between versions        | N/A — switched to netlink in Phase 2; no JSON dependency                 |
| Polling `nft -j` is expensive on huge rulesets  | Netlink + monitor events + 2 s default. 50k-rule scan benchmarks at ~2 ms |
| Bad commit locks user out over SSH              | Phase 5.2 dead-man's switch fires after 60 s if not confirmed             |
| Form editor can't represent every nft feature   | Form covers common cases; F8 raw mode always available; unknown verbatim |
| `tview` API churn                               | Pinned in `go.mod`; vendor on release if churn becomes a problem          |

One risk we didn't anticipate: `google/nftables` v0.3.0 has a bug that
writes verdict-map data-type into `KeyType` instead of `DataType`.
We caught it in Phase 2.6 and pinned to a HEAD revision that fixes it.
Pin to `v0.3.1+` once it's tagged.
