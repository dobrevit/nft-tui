# nft-tui

A terminal UI for **inspecting and managing nftables** on Linux, written in Go.

Inspired by `k9s`, `htop`, and Midnight Commander: a multi-pane, keyboard-driven
view of the live ruleset that lets a firewall administrator navigate from
*ruleset → table → chain → rule* without leaving the terminal, then stage and
commit changes safely.

## Status

**v1.0 ready.** All six phases of the roadmap are shipped. The full
read → edit → dry-run → commit → snapshot → restore loop works
end-to-end against the real kernel; see
[docs/06-roadmap.md](docs/06-roadmap.md#v10---ready) for the release
summary and [docs/07-deferred.md](docs/07-deferred.md) for known
follow-ups.

## Highlights

- **Netlink read path** via [`google/nftables`](https://github.com/google/nftables) — no `nft` shell-out for inspection
- **Live counters** refreshed every 2 s (tunable), plus an opt-in `NFT_MSG_NEW*`
  subscription that refreshes within 250 ms of any external change
- **Tree explorer + rule table** with configurable column presets, `/` filter,
  `:` global search, `y` yank to clipboard via OSC 52
- **Live monitor** (`m`) — top-N rules by pps / bps / Δpkts, per-rule
  Unicode-block sparkline over the last ~2 min
- **Staged edits** (`--write`): `a` add, `o`/`O` insert, `e` edit, `d` delete;
  `D` opens the diff page; F3 dry-runs via `nft -c`, F2 commits via `nft -f`
- **Form + raw editor**, F8 toggles. The form covers the common-case fields;
  raw mode handles 100% of nftables
- **Snapshot / restore** via `:w <path>` / `:r <path>`. Restore is guarded by a
  60-second dead-man's switch that auto-rolls-back if not confirmed — recovers
  from SSH-lockout scenarios automatically
- **Audit log** in `$XDG_STATE_HOME/nft-tui/` — per-commit nft files plus a
  rolling `audit.log` with timestamp / UID / username / action / payload
- **Themes** (`default`, `high-contrast`, `mono`) via `--theme`
- **Single static binary** (`CGO_ENABLED=0`); `.deb` / `.rpm` / `tar.gz` builds
  via goreleaser; groff man page

## Quick start

Read-only inspection:

```sh
$ sudo nft-tui
```

With editing enabled:

```sh
$ sudo nft-tui -write
```

Dump the parsed ruleset (no TUI, pipe-friendly):

```sh
$ sudo nft-tui -dump | grep "dport 22"
```

Development without privileges, inside an unshared user/net namespace:

```sh
$ unshare -rn ./nft-tui
```

See `nft-tui -help` and `man nft-tui` for the full reference.

## Build

```sh
make build         # → ./nft-tui (static, CGO disabled)
make test          # unit tests
make integration   # integration tests inside unshare -rn (needs nft binary)
make install       # → $PREFIX/bin/nft-tui + $PREFIX/share/man/man1/nft-tui.1
```

For release tagging, push a `vX.Y.Z` tag and run `goreleaser release --clean`
(see `.goreleaser.yaml`).

## Quick links

- [Product brief](docs/01-product-brief.md) — who, what, why
- [Architecture](docs/02-architecture.md) — netlink reads, `nft -f` writes
- [Screen designs](docs/03-screens.md) — ASCII mockups of every view
- [Keybindings](docs/04-keybindings.md) — the keymap, in one page
- [Data model](docs/05-data-model.md) — how nftables maps to UI state
- [Roadmap](docs/06-roadmap.md) — phased delivery (all phases shipped)
- [Deferred work](docs/07-deferred.md) — what was punted and why

## Non-goals

- A configuration generator for people who don't already know nftables. The
  audience is admins who *can* write `nft` by hand but want a faster way to
  inspect, diff, and edit a running ruleset.
- A `firewalld` / `ufw` replacement. We target raw nftables; higher-level
  abstractions are out of scope.
- A daemon. `nft-tui` is a short-lived interactive process. Persistent rule
  storage is whatever the OS already does (`/etc/nftables.conf`, systemd).
- Multi-host fanout. If you want to manage 50 routers at once, use Ansible to
  push `nft` files; this tool is one host at a time.
