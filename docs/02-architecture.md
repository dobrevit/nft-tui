# Architecture & library choices

## High-level shape

```
                ┌──────────────────────────────────────────┐
                │              TUI layer (tview)           │
                │  panes · forms · modals · key dispatcher │
                └───────────────┬──────────────────────────┘
                                │  view models (Go structs)
                ┌───────────────┴──────────────────────────┐
                │            App / state layer            │
                │  ruleset cache · diff buffer · undo log  │
                └───────────────┬──────────────────────────┘
                                │
                ┌───────────────┴──────────────────────────┐
                │           nftables adapter               │
                │  read:  netlink via google/nftables      │
                │  render: own nft-syntax renderer         │
                │  write: netlink batches (atomic)         │
                └───────────────┬──────────────────────────┘
                                │
                             Linux kernel (nf_tables)
```

Three layers, deliberately. The TUI never talks to the kernel directly; the
adapter never knows about widgets. This makes the read path testable with
fixture JSON and the write path swappable (CLI vs. netlink) without UI churn.

## TUI framework: `tview` + `tcell`

The user asked about ncurses. In Go the practical ncurses-equivalent stack is
[`rivo/tview`](https://github.com/rivo/tview) built on
[`gdamore/tcell`](https://github.com/gdamore/tcell). `tcell` is a pure-Go
terminal library covering the same ground as ncurses (capabilities, colour,
mouse, resize, alt-screen). `tview` adds the high-level widgets we need.

**Why not raw ncurses (cgo)?** It works (e.g. `gbin/goncurses`) but drags in a
C dependency, complicates cross-compilation and static linking, and gives us
nothing tview doesn't already provide.

**Why not `bubbletea`?** It's lovely for single-purpose flows, but the
Elm-style model becomes verbose for a multi-pane explorer with a deep nested
data model and modal forms. `tview` is closer in spirit to ncurses
applications like `htop` or `mc`, which is the look-and-feel we want.

### Widgets we'll lean on

| Widget                 | Used for                                            |
|------------------------|-----------------------------------------------------|
| `tview.TreeView`       | Left-pane navigation (ruleset → table → chain)      |
| `tview.Table`          | Rule list with sortable columns and counters        |
| `tview.Flex`           | Pane layout                                         |
| `tview.Form`           | Rule editor (typed expression / statement builders) |
| `tview.Modal`          | Confirmations, error toasts                         |
| `tview.TextView`       | Diff view, raw `nft` output, help                   |
| `tview.InputField`     | Search / filter / command line                      |
| `tview.Pages`          | Stacking modal "screens"                            |

## nftables adapter

**Decision (2026-05-22): netlink-only read path via `google/nftables`.** No
shell-out to `nft` for normal operation. Rationale: pure Go, no fork-per-tick,
gives us the structured expression AST directly, and supports change
monitoring (`NFT_MSG_NEW*`) so we can drop polling later.

The trade-off is that `google/nftables` exposes the **binary expression AST**
(`expr.Meta`, `expr.Payload`, `expr.Cmp`, `expr.Counter`, `expr.Verdict`, …)
but does **not** ship a textual `nft` renderer. We own the renderer:
`internal/nft/render.go` walks the expression list and emits canonical nft
syntax. Unrecognised expressions fall back to a typed placeholder
(`<expr:bitwise>`) so we never lie about what's in the kernel.

### Read

- Subscribe / list via `github.com/google/nftables`.
- Counters are read with each chain (or as named counter objects); refresh
  is either timer-tick or netlink event (when subscription is wired in
  Phase 4).
- For development on a host without `CAP_NET_ADMIN`, run inside an unshared
  user/network namespace: `unshare -rn nft-tui`. Empty ruleset, but the
  netlink calls work.

### Write (Phase 3 — open question)

Two viable approaches; final call deferred to Phase 3 kick-off:

- **Netlink batches** via `Conn.Flush()`. Atomic per batch (one
  `NFT_MSG_BEGIN`/`NFT_MSG_END`), pure Go, no shell-out, but we lose the
  "audit log is a file admins can paste into Ansible" property unless we
  also emit the equivalent nft text.
- **Shell out to `nft -f <staged-file>`**. Atomic by virtue of nft's own
  transaction, gives a human-readable audit trail for free, and `nft -c`
  is a battle-tested validator. But it splits the adapter between two
  mechanisms.

Likely answer: **netlink for the actual commit, with our renderer also
producing an `nft` text artefact written next to the audit log.** That
preserves the "single source of truth in nft syntax" property without
forking a subprocess on the commit hot path.

For Phase 2 (read-only) this isn't on the critical path.

### Permissions

Read needs `CAP_NET_ADMIN` for netlink, or simply `nft list ruleset` which
typically also needs root. We don't try to drop privileges mid-process; the
tool runs at the privilege of the invoking user and degrades gracefully
(read-only on EACCES, no monitor if netlink subscribe fails).

## State management

The app keeps an in-memory **`Ruleset`** value (immutable snapshot) plus a
**`StagedChange`** list. Every edit appends to the staged list; the diff view
renders `apply(snapshot, staged)` vs `snapshot`. Commit serialises the staged
list to nft syntax, writes a temp file, runs `nft -c`, then `nft -f`.

Undo = pop the staged list. After commit, the staged list is cleared and the
ruleset is re-fetched.

## Concurrency

- One goroutine for the UI event loop (tview owns this).
- One goroutine for the polling refresh; sends a new `Ruleset` over a channel.
- One goroutine for any in-flight `nft` invocation (commit, dry-run).
- All channel sends back to the UI go through `App.QueueUpdateDraw` so we
  never touch widgets off the UI thread.

## Repo layout (proposed)

```
nft-tui/
  cmd/nft-tui/          main.go — flag parsing, app bootstrap
  internal/nft/         adapter: parse `nft -j`, emit `nft` syntax, netlink
  internal/model/       Ruleset, Table, Chain, Rule, Set, Counter
  internal/state/       App state, staged changes, undo log
  internal/ui/          tview widgets, one file per screen
  internal/ui/keys/     keymap & dispatcher
  internal/diff/        ruleset diff (text + structured)
  testdata/             fixture JSON from `nft -j list ruleset`
  docs/                 this directory
```

## Dependencies (initial)

- `github.com/rivo/tview`
- `github.com/gdamore/tcell/v2`
- `github.com/google/nftables` (optional read path)
- `github.com/spf13/cobra` (subcommands: `nft-tui`, `nft-tui snapshot`, etc.)
- stdlib for everything else (JSON, os/exec, log/slog)

No web framework, no ORM, no plugin system.
