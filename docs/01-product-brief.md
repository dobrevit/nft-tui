# Product brief

## Problem

Linux administrators managing nftables today have two options:

1. **`nft list ruleset`** — dumps the entire ruleset as text. Fine for small
   setups, unreadable past a few hundred rules. No navigation, no filtering,
   no live counters, and editing means hand-writing `nft add rule …` or
   re-loading an entire file.
2. **Higher-level wrappers** (`firewalld`, `ufw`, web UIs from appliance
   vendors). They hide nftables semantics, which is exactly what experienced
   admins *don't* want.

There's a missing middle: a tool that exposes nftables natively, but with the
ergonomics of a modern TUI — tree navigation, filtering, live counters,
stage-and-commit edits, and undo.

## Audience

- **Primary**: Linux sysadmins, SREs, and network engineers who already write
  nftables by hand and run it on servers, routers, or appliances.
- **Secondary**: Security engineers auditing a ruleset on a box they didn't
  set up, who need to find "what's hitting this chain right now."
- **Out of scope**: Desktop end users wanting a friendly firewall. They are
  better served by `firewalld`'s GUI or distro tooling.

## Goals (Phase 1 ships design only)

1. Read-only **explorer**: ruleset → table → chain → rule, with packet/byte
   counters refreshed live.
2. **Filter & search** across the whole ruleset (by interface, address, port,
   chain, comment).
3. **Stage edits** in a local buffer; show a diff against the live ruleset;
   commit atomically via `nft -f` or netlink transaction.
4. **Dry-run validation** before commit (`nft -c -f`).
5. **Snapshot & restore** the current ruleset to/from a file, so an admin
   can roll back a bad commit.
6. **No surprises**: every commit is logged with the exact nftables syntax
   that was applied, so the user can paste it into their config-management
   system of choice.

## Non-goals

- Generating rules from a wizard / high-level intent ("block all SSH from
  China"). Admins write nftables; we render and edit it.
- Managing `iptables` / `ipset` legacy stacks. nftables only.
- Network-wide orchestration. One host at a time.
- A long-running daemon, an API server, or a web UI.

## Success criteria

- An admin can find the rule that's currently matching traffic on a busy
  forwarding host in under 10 seconds.
- An admin can edit a rule, see a diff, and commit it without ever leaving
  the TUI — and without breaking the existing ruleset if their edit is
  syntactically wrong.
- The tool runs on a stock server with no dependencies beyond `nft` (and
  optionally `CAP_NET_ADMIN` for direct netlink mode).

## Constraints

- Must run over SSH on a 80×24 terminal as a minimum, scaling up gracefully.
- Must be safe by default: read-only mode is the default, write mode requires
  an explicit flag (`--write`) or runtime toggle.
- Must not require Python, Node, or any non-Go runtime dependencies. Single
  static binary.
