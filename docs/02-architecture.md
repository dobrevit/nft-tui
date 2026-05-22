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
                │  read: netlink (google/nftables) OR      │
                │        `nft -j list ruleset`             │
                │  write: `nft -f <staged>` (transaction)  │
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

Two read paths, one write path.

### Read

- **Primary**: shell out to `nft -j list ruleset`. JSON is well-defined,
  works on any host with `nft` ≥ 0.9, and matches what admins already debug
  with. Parse it into the data model in [05-data-model.md](05-data-model.md).
- **Optional fast path**: [`google/nftables`](https://github.com/google/nftables),
  a pure-Go netlink client. Avoids forking `nft` on every refresh and supports
  monitoring (`NFT_MSG_NEW*` events). Behind a `--netlink` flag in v1; default
  later once it's proven.

Counters are refreshed by re-running `nft -j list ruleset` on a tick (default
2 s, tunable). With netlink we can subscribe and avoid the poll.

### Write

- **Always** through `nft -f <staged-file>`. Reasons:
  1. nft parses & validates the entire staged ruleset atomically; partial
     commits are impossible.
  2. The staged file is the audit log — we keep it after commit.
  3. Admins can copy/paste it into config management (Ansible, etc.).
- **Dry-run**: `nft -c -f <staged-file>` before commit. Errors surface in the
  diff/commit screen with line numbers.
- **Snapshot**: `nft list ruleset > <path>`; restore is `nft -f <path>` after
  `nft flush ruleset` (gated behind a scary confirmation).

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
