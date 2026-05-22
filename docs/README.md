# nft-tui design docs

Phase 1 deliverable: design only. Read in order.

1. [Product brief](01-product-brief.md) — problem, audience, goals, non-goals
2. [Architecture](02-architecture.md) — `tview`/`tcell` over ncurses, `nft -j`
   read path, atomic-commit write path, repo layout
3. [Screen designs](03-screens.md) — ASCII mockups of every view, with the
   reasoning behind each layout
4. [Keybindings](04-keybindings.md) — the keymap on one page
5. [Data model](05-data-model.md) — how nftables maps to Go types and how
   staged changes are represented
6. [Roadmap](06-roadmap.md) — phased delivery, risks, what's out of scope
