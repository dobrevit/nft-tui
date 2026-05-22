# nft-tui

A terminal UI for **visualising and managing nftables** on Linux, written in Go.

Inspired by `k9s`, `htop`, and Midnight Commander: a multi-pane, keyboard-driven
view of the live ruleset that lets a firewall administrator navigate from
*ruleset → table → chain → rule* without leaving the terminal, then stage and
commit changes safely.

## Status

**Phase 1 — Product & UX design.** This repo currently contains documentation
and ASCII mockups only. No Go code yet. See [docs/](docs/) for the brief,
architecture, screen designs, data model, keybindings, and roadmap.

## Quick links

- [Product brief](docs/01-product-brief.md) — who, what, why
- [Architecture & library choice](docs/02-architecture.md) — tview vs. bubbletea, netlink vs. CLI
- [Screen designs](docs/03-screens.md) — ASCII mockups of every view
- [Data model](docs/05-data-model.md) — how nftables maps to UI state
- [Keybindings](docs/04-keybindings.md) — the keymap, in one page
- [Roadmap](docs/06-roadmap.md) — phased delivery plan

## Non-goals

- A configuration generator for people who don't already know nftables. The
  audience is admins who *can* write `nft` by hand but want a faster way to
  inspect, diff, and edit a running ruleset.
- A `firewalld` / `ufw` replacement. We target raw nftables; higher-level
  abstractions are out of scope.
- A daemon. `nft-tui` is a short-lived interactive process. Persistent rule
  storage is whatever the OS already does (`/etc/nftables.conf`, systemd).
