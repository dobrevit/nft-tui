# Screen designs

ASCII mockups of every screen, sized to 100×30 (gracefully degrades to 80×24
by collapsing the right pane). Box characters use the Unicode line set; on a
real terminal they render as solid lines.

Conventions:

- `[F1]`-style brackets denote function-key hints in the status bar.
- `▸` / `▾` are collapsed / expanded tree nodes.
- `●` is a live counter that's updating; `○` is stale (no traffic since
  last poll).
- The breadcrumb at the top shows the current navigation path.
- The bottom line is the **command line** (vim-style `:` prefix) and the
  **mode indicator** (RO / RW / STAGED).

---

## 1. Main explorer (default screen)

```
┌─ nft-tui ── host: edge-router-01 ──────────────────────── ruleset @ 14:02:11 ●─┐
│ Ruleset › inet › filter › forward                                              │
├──────────────────────┬─────────────────────────────────────────────────────────┤
│ ▾ inet filter        │  # chain forward { type filter hook forward prio 0; }   │
│   ▾ chains           │                                                         │
│     ▸ input          │  H#  PKTS    BYTES   RULE                               │
│     ▸ output         │  ──  ──────  ──────  ───────────────────────────────── │
│     ▾ forward    ●   │   3  1.2M    890M    ct state established,related accept│
│       3 rules        │   5    412    32K    iifname "eth0" oifname "eth1" \    │
│     ▸ prerouting     │                       ip saddr 10.0.0.0/24 accept       │
│   ▸ sets (2)         │   7      0      0    log prefix "drop " counter drop    │
│   ▸ maps (1)         │                                                         │
│   ▸ flowtables (1)   │  policy: drop                                           │
│ ▸ ip nat             │                                                         │
│ ▸ ip6 filter         │  Counters refreshed every 2.0s (last: 14:02:11)         │
│ ▸ bridge filter      │                                                         │
├──────────────────────┴─────────────────────────────────────────────────────────┤
│ /search   :command   [Enter] open   [e] edit   [a] add rule   [d] delete   [?]│
│ MODE: RO ── staged: 0 ── kernel: 6.8.0 ── nft: 1.0.9                          │
└────────────────────────────────────────────────────────────────────────────────┘
```

**Why this layout.** Left pane is a tree (the natural nftables hierarchy).
Right pane shows the *contents* of the selected node — for a chain that's the
rule list with counters; for a set it's set elements; for a table it's a
summary. This is the same idiom as `k9s` and `mc`, which experienced
sysadmins already have muscle memory for.

The header is intentionally one line — every column visible at 80 cols.

---

## 2. Rule list (chain selected, right pane focused)

```
┌─ Ruleset › inet › filter › forward ──────────────────────────────────────────┐
│ H#  PKTS      BYTES    IIF     OIF     SRC              DST            VERDICT│
│ ─── ─────── ──────── ─────── ─────── ──────────────── ─────────────── ────── │
│   3 1.2M     890M    *       *       *                *               accept │
│   5   412     32K    eth0    eth1    10.0.0.0/24      *               accept │
│   7     0      0     *       *       *                *               drop ● │
│   9    18    1.4K    eth1    *       *                fe80::/10       drop   │
│  11 4.7K    310K     *       *       *                *               jump LOG│
│                                                                              │
│ ▸ rule #5: iifname "eth0" oifname "eth1" ip saddr 10.0.0.0/24 counter accept │
│   added:    by admin@host on 2026-04-18                                      │
│   comment:  "internal LAN to DMZ"                                            │
│   counter:  pkts 412 bytes 32768 (since 14:00:09)                            │
└──────────────────────────────────────────────────────────────────────────────┘
 [j/k] move  [Enter] details  [e] edit  [a] add  [d] delete  [y] yank syntax
 [/] filter  [s] sort  [r] reset counters  [Esc] back to tree
```

**Why this layout.** Counters are the most-asked-for column when triaging,
so they sit at the left. The flattened "src / dst / verdict" columns are
derived from the rule's expressions; the unabbreviated rule appears in the
inspector below so we never lie about what's actually in the kernel.

`y` yanks the canonical `nft` syntax to the system clipboard (or, over SSH,
the OSC 52 sequence) — admins frequently want to paste a rule into their
config-management repo.

---

## 3. Rule editor (modal, on `e` or `a`)

```
   ┌────────────── Edit rule #5 in inet filter forward ──────────────┐
   │                                                                 │
   │  ┌─ Match ──────────────────────────────────────────────────┐   │
   │  │ Interface in  : [eth0                              ▾]    │   │
   │  │ Interface out : [eth1                              ▾]    │   │
   │  │ Source        : [10.0.0.0/24                          ]  │   │
   │  │ Destination   : [                                     ]  │   │
   │  │ Protocol      : [( ) tcp  ( ) udp  ( ) icmp  (•) any ]   │   │
   │  │ Sport / Dport : [          ] / [          ]              │   │
   │  │ ct state      : [x] established  [x] related  [ ] new    │   │
   │  └──────────────────────────────────────────────────────────┘   │
   │  ┌─ Action ─────────────────────────────────────────────────┐   │
   │  │ Verdict   : (•) accept  ( ) drop  ( ) reject  ( ) jump   │   │
   │  │ Log       : [ ] enabled   prefix [                    ]  │   │
   │  │ Counter   : [x] enabled                                  │   │
   │  │ Comment   : [internal LAN to DMZ                       ] │   │
   │  └──────────────────────────────────────────────────────────┘   │
   │                                                                 │
   │  Preview (nft syntax):                                          │
   │  ┌───────────────────────────────────────────────────────────┐  │
   │  │ iifname "eth0" oifname "eth1" ip saddr 10.0.0.0/24 \      │  │
   │  │   ct state { established, related } counter accept \      │  │
   │  │   comment "internal LAN to DMZ"                           │  │
   │  └───────────────────────────────────────────────────────────┘  │
   │                                                                 │
   │   [F5] Stage   [F6] Stage & open diff   [F8] Raw mode   [Esc]   │
   └─────────────────────────────────────────────────────────────────┘
```

**Why this layout.** Two-section form (Match / Action) mirrors the mental
model of an nftables rule. The live `nft`-syntax preview is the single
source of truth — if a field is unsupported in the form, the user hits
**F8 Raw mode** and edits the syntax directly in a text area. We never
hide what's actually being committed.

`F5` only stages; `F6` stages and jumps to the diff/commit screen so the
admin can review and apply immediately.

---

## 4. Diff / commit screen

```
┌─ Staged changes (3) ─────────────────────────────────────────────────────────┐
│  ▾ inet filter forward                                                       │
│      ~ rule #5  (modified)                                                   │
│      + rule     (added, will get handle on commit)                           │
│  ▾ inet filter input                                                         │
│      - rule #11 (deleted)                                                    │
├──────────────────────────────────────────────────────────────────────────────┤
│  - iifname "eth0" oifname "eth1" ip saddr 10.0.0.0/24 counter accept         │
│  + iifname "eth0" oifname "eth1" ip saddr 10.0.0.0/24 \                      │
│  +   ct state { established, related } counter accept \                      │
│  +   comment "internal LAN to DMZ"                                           │
│                                                                              │
│  + add rule inet filter forward tcp dport { 22, 80, 443 } accept             │
│                                                                              │
│  - delete rule inet filter input handle 11                                   │
├──────────────────────────────────────────────────────────────────────────────┤
│  Dry-run (nft -c -f staged.nft):  ✓ OK                                       │
│  Snapshot before commit:          /var/lib/nft-tui/snap-2026-05-22T14-02.nft │
│                                                                              │
│  [F2] Commit   [F3] Dry-run again   [F4] Edit raw   [u] Unstage   [Esc] Back │
└──────────────────────────────────────────────────────────────────────────────┘
```

**Why this layout.** Three zones: *what's staged* (tree), *the diff* (unified
+/- text), *safety info* (dry-run result, snapshot path). The admin sees
exactly what will be applied, in nft syntax, before pressing F2.

If dry-run fails the green ✓ becomes a red ✗ with the parser error inline,
and F2 is disabled.

---

## 5. Sets & maps view

```
┌─ Ruleset › inet › filter › sets ─────────────────────────────────────────────┐
│ NAME              TYPE                FLAGS             SIZE     TIMEOUT      │
│ ─────────────── ─────────────────── ─────────────────── ─────── ───────────── │
│ blacklist_v4    ipv4_addr           dynamic, timeout    1,284   1h            │
│ admin_ports     inet_service        constant               12   —             │
│ trusted_macs    ether_addr          interval                5   —             │
├──────────────────────────────────────────────────────────────────────────────┤
│ blacklist_v4 elements (1,284):                                               │
│   203.0.113.42        expires 00:42:11                                       │
│   198.51.100.7        expires 00:38:02                                       │
│   192.0.2.250         expires 00:12:55                                       │
│   …  [g] go to top   [G] bottom   [/] filter   [a] add element   [d] delete │
└──────────────────────────────────────────────────────────────────────────────┘
```

**Why this layout.** Sets are first-class in nftables and admins live in them
(threat-intel feeds, port allowlists). They need their own pane, not a popup.

---

## 6. Live monitor / counters

```
┌─ Live monitor ───────────────────────────────────────────────────────────────┐
│ Interval: 1.0s     Top: by pps     Hosts: forwarding-active only             │
│                                                                              │
│ CHAIN          H#   PPS      BPS       Δ-PKTS   RULE                         │
│ ──────────── ──── ─────── ──────── ──────── ────────────────────────────────│
│ forward         3   8.4K   62 MB/s   +8.4K   ct state established accept     │
│ forward         5    120    1.1 MB/s   +120   iifname eth0 oifname eth1 …    │
│ input          22     12    1.0 KB/s    +12   tcp dport 22 accept            │
│ forward         7      4     320 B/s    +4    log prefix "drop " counter drop│
│                                                                              │
│ ──── sparkline (last 60s, forward chain total pps) ────                      │
│  8K │              ▂▃▄▆▇█▇▆▅▄▃▂▁ ▁▂▃▄▅▆▇█▇▆▅▄▃▂▁▁                            │
│  4K │ ▂▃▄▅▆▇█▇▆▅▄ ▁                                  ▁▂▃▄▅▆▇█▇               │
│  0  └────────────────────────────────────────────────────────────────────    │
│         -60s                                                              now │
└──────────────────────────────────────────────────────────────────────────────┘
 [p] pause  [+/-] interval  [s] sort  [f] filter chain  [Esc] back
```

**Why this layout.** `htop`-style top-N table sorted by pps, with a sparkline
for the busiest chain. Triage view — "what's hammering this box right now?"

---

## 7. Search / command palette

```
   ┌─ Search ──────────────────────────────────────────────────────────────┐
   │ :  saddr 10.0.0.0/24                                                  │
   ├───────────────────────────────────────────────────────────────────────┤
   │  inet filter forward    rule #5    ip saddr 10.0.0.0/24 …             │
   │  inet filter forward    rule #14   ip saddr 10.0.0.0/24 tcp dport 53  │
   │  inet nat   postrouting rule #2    ip saddr 10.0.0.0/24 masquerade    │
   │  set blacklist_v4       contains   10.0.0.7   (1 match)               │
   │                                                                       │
   │  4 matches across 2 tables, 3 chains, 1 set                           │
   └───────────────────────────────────────────────────────────────────────┘
    [Enter] jump   [Tab] next   [Ctrl-G] grep mode (regex)   [Esc] cancel
```

Triggered by `/` (incremental filter inside the current view) or `:`
(global search across the whole ruleset).

---

## 8. Help overlay

```
   ┌─ Help (press ? again to close) ───────────────────────────────────────┐
   │                                                                       │
   │  Navigation                       Editing                             │
   │  ─────────                        ─────────                           │
   │  j / ↓     down                   e          edit selected            │
   │  k / ↑     up                     a          add (rule/set/chain)     │
   │  h / ←     collapse / left pane   d          delete                   │
   │  l / →     expand  / right pane   u          unstage last change      │
   │  g / G     top / bottom           U          unstage all              │
   │  Tab       next pane              y          yank nft syntax          │
   │                                   F8         raw-syntax editor        │
   │  Filter & search                                                      │
   │  ───────────────                  Commit                              │
   │  /         filter current view    F2         commit staged            │
   │  :         global command         F3         dry-run                  │
   │  Ctrl-G    regex search           F5         stage edit (in form)     │
   │                                                                       │
   │  Modes                            Files                               │
   │  ─────                            ─────                               │
   │  r        toggle read/write       :w <path>  snapshot ruleset         │
   │  m        live monitor            :r <path>  restore ruleset (!)      │
   │                                   :e <path>  edit external nft file   │
   │                                                                       │
   │                       Press ? or Esc to close                         │
   └───────────────────────────────────────────────────────────────────────┘
```

---

## 9. Snapshot / restore confirmation

```
   ┌─ Restore ruleset from snapshot ───────────────────────────────────────┐
   │                                                                       │
   │   File: /var/lib/nft-tui/snap-2026-05-22T14-02.nft                    │
   │   Size: 28 KB · 4 tables · 17 chains · 142 rules                      │
   │                                                                       │
   │   This will:                                                          │
   │     1. flush the current ruleset                                      │
   │     2. apply the snapshot atomically (nft -f)                         │
   │                                                                       │
   │   ⚠  If you are connected over the network this can lock you out.     │
   │      A dead-man's switch will revert in 60s unless you confirm        │
   │      again inside the TUI.                                            │
   │                                                                       │
   │              [ Cancel ]              [ I understand, proceed ]        │
   └───────────────────────────────────────────────────────────────────────┘
```

**Why this exists.** Restoring is the most dangerous action in the tool.
The 60-second dead-man's switch (a `flush ruleset` + `nft -f <old>` job
scheduled with `at(1)` or an in-process timer, cancelled by a second
confirmation) is the same pattern Cisco uses for `reload in 5`. It's worth
the engineering cost because it's the difference between "oops" and
"drive to the datacentre."

---

## 10. Startup / no-ruleset edge case

```
   ┌─ nft-tui ─────────────────────────────────────────────────────────────┐
   │                                                                       │
   │   The kernel ruleset is empty.                                        │
   │                                                                       │
   │   You can:                                                            │
   │     [n] start a new ruleset interactively                             │
   │     [l] load /etc/nftables.conf                                       │
   │     [o] open another file…                                            │
   │     [q] quit                                                          │
   │                                                                       │
   │   nft-tui is in read-only mode (--write not given).                   │
   │   Use --write to enable editing.                                      │
   │                                                                       │
   └───────────────────────────────────────────────────────────────────────┘
```
