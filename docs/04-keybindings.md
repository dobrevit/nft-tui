# Keybindings

vim-style by default, with function-key shortcuts for actions that take a
hand off the home row. Discoverable via `?` from any screen.

## Global

| Key       | Action                                       |
|-----------|----------------------------------------------|
| `?`       | Help overlay (toggle)                        |
| `q`       | Quit (warns if there are staged changes)     |
| `Ctrl-C`  | Hard quit (discards staged changes)          |
| `Tab`     | Cycle pane focus                             |
| `Esc`     | Back / close modal                           |
| `:`       | Global command line                          |
| `/`       | Filter current view                          |
| `Ctrl-G`  | Regex search across whole ruleset            |
| `r`       | Toggle read-only / read-write mode           |
| `m`       | Live monitor screen                          |
| `F1`      | Help                                         |
| `F12`     | Cycle colour theme                           |

## Tree (left pane)

| Key             | Action                              |
|-----------------|-------------------------------------|
| `j` / `↓`       | Move down                           |
| `k` / `↑`       | Move up                             |
| `h` / `←`       | Collapse node / move to parent      |
| `l` / `→`       | Expand node / enter right pane      |
| `Enter`         | Open node in right pane             |
| `g` / `G`       | Top / bottom                        |
| `Ctrl-D` / `-U` | Half-page down / up                 |

## Rule list (right pane)

| Key       | Action                                              |
|-----------|-----------------------------------------------------|
| `j`/`k`   | Move row                                            |
| `Enter`   | Expand inspector for selected rule                  |
| `e`       | Edit rule (form)                                    |
| `a`       | Add rule before / after (asks which)                |
| `d`       | Delete rule (asks confirm)                          |
| `y`       | Yank canonical `nft` syntax to clipboard / OSC 52   |
| `s`       | Cycle sort column (handle / pkts / bytes)           |
| `S`       | Reverse sort                                        |
| `r`       | Reset counters on selected rule                     |

## Editor (modal)

| Key       | Action                                              |
|-----------|-----------------------------------------------------|
| `Tab`     | Next field                                          |
| `Shift-Tab` | Previous field                                    |
| `F5`      | Stage change                                        |
| `F6`      | Stage and open diff/commit                          |
| `F8`      | Toggle raw `nft`-syntax editor                      |
| `Esc`     | Cancel (asks if there are unsaved fields)           |

## Diff / commit

| Key       | Action                                              |
|-----------|-----------------------------------------------------|
| `j`/`k`   | Move within diff                                    |
| `u`       | Unstage last change                                 |
| `U`       | Unstage all                                         |
| `F2`      | Commit (after a successful dry-run)                 |
| `F3`      | Re-run dry-run                                      |
| `F4`      | Open staged file in `$EDITOR`                       |

## Live monitor

| Key       | Action                          |
|-----------|---------------------------------|
| `p`       | Pause / resume                  |
| `+` / `-` | Increase / decrease interval    |
| `s`       | Cycle sort (pps / bps / Δpkts)  |
| `f`       | Filter to one chain             |

## Command line (`:` prefix)

| Command             | Action                                              |
|---------------------|-----------------------------------------------------|
| `:w <path>`         | Write ruleset snapshot to path                      |
| `:r <path>`         | Restore ruleset from snapshot (dangerous, confirms) |
| `:e <path>`         | Open external `nft` file in editor                  |
| `:commit`           | Same as F2                                          |
| `:flush <table>`    | Flush a table (stages, doesn't apply directly)      |
| `:reload`           | Re-read kernel ruleset, discard cache               |
| `:set monitor on`   | Enable netlink monitoring (if available)            |
| `:q` / `:q!`        | Quit / force quit                                   |
