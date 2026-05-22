# Keybindings

vim-style by default, with function-key shortcuts for actions that take a
hand off the home row. Discoverable via `?` from any screen, and documented
in `man nft-tui`.

## Global

| Key       | Action                                       |
|-----------|----------------------------------------------|
| `?`       | Help overlay (toggle)                        |
| `q`       | Quit                                         |
| `Ctrl-C`  | Hard quit                                    |
| `Tab`     | Cycle pane focus (tree ↔ rules)              |
| `Esc`     | Back / close modal                           |
| `:`       | Global search OR `:w <path>` / `:r <path>`   |
| `/`       | Filter current rule list                     |
| `y`       | Yank selected rule's nft syntax (OSC 52)     |
| `R`       | Full reload                                  |
| `m`       | Live monitor                                 |
| `D`       | Diff / commit page                           |
| `c`       | Cycle rule-list column preset                |

## Tree (left pane)

| Key             | Action                              |
|-----------------|-------------------------------------|
| `j` / `↓`       | Move down                           |
| `k` / `↑`       | Move up                             |
| `h` / `←`       | Collapse node / move to parent      |
| `l` / `→`       | Expand node                         |
| `Enter`         | Open node in right pane             |
| `g` / `G`       | Top / bottom                        |

## Rule list (right pane, requires `--write` for edits)

| Key       | Action                                              |
|-----------|-----------------------------------------------------|
| `j` / `k` | Move row                                            |
| `a`       | Add (append) rule to the current chain              |
| `o` / `O` | Insert AFTER / BEFORE the selected rule             |
| `e`       | Edit (replace) the selected rule                    |
| `d`       | Stage a delete of the selected rule                 |
| `y`       | Yank canonical `nft` syntax via OSC 52              |

## Editor page

| Key         | Action                                            |
|-------------|---------------------------------------------------|
| `Tab`       | Next field (between body, comment, form fields)   |
| `F5`        | Stage and close                                   |
| `F6`        | Stage and open the diff page                      |
| `F8`        | Toggle between form view and raw-nft TextArea     |
| `Esc`       | Cancel                                            |

## Diff / commit page

| Key       | Action                                              |
|-----------|-----------------------------------------------------|
| `j` / `k` | Move within the summary                             |
| `u`       | Unstage last change                                 |
| `U`       | Unstage all                                         |
| `F3`      | Dry-run via `nft -c -f`                             |
| `F2`      | Commit via `nft -f` (opens Apply/Cancel modal,      |
|           | enabled only after a passing F3)                    |
| `Esc`     | Back to the explorer                                |

## Live monitor

| Key       | Action                                |
|-----------|---------------------------------------|
| `s`       | Cycle sort metric (pps / bps / Δpkts) |
| `p`       | Pause / resume                        |
| `j` / `k` | Move row (updates the sparkline)      |
| `Esc`     | Back to the explorer                  |

## Dead-man's switch (restore confirmation)

| Key   | Action                                         |
|-------|------------------------------------------------|
| `Y`   | Apply / keep new ruleset                       |
| `Esc` | Cancel / roll back to pre-restore state        |
| —     | 60-second timeout auto-rolls-back              |

## Command line (`:` prefix)

| Command             | Action                                              |
|---------------------|-----------------------------------------------------|
| `:w <path>`         | Snapshot the live ruleset to `<path>`               |
| `:write <path>`     | (long form)                                         |
| `:r <path>`         | Restore from snapshot (60-second dead-man's switch) |
| `:read <path>`      | (long form)                                         |
| `:restore <path>`   | (long form)                                         |
| Anything else       | Treated as a global-search query                    |

Paths starting with `~/` are expanded against `$HOME`.

## Commands not yet implemented

These were listed in the original Phase 1 keymap but didn't ship in v1.0
(left out because the existing affordances cover the use case):

- `Ctrl-G` regex search — `/` already does case-insensitive substring,
  which has been enough for our test rulesets
- `F1` help — `?` does the job
- `F12` theme cycle — themes are picked at startup via `--theme`
- `:e <path>` external-editor pivot
- `:commit`, `:flush <table>`, `:reload`, `:set monitor on`
- Live-monitor: `+`/`-` interval, `f` chain filter

If you need any of these, open an issue.
