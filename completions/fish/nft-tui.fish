# fish completion for nft-tui.
#
# Install: drop this file at /usr/share/fish/vendor_completions.d/nft-tui.fish
# (the .deb / .rpm do that automatically). Manual install:
#
#     install -Dm0644 nft-tui.fish ~/.config/fish/completions/nft-tui.fish
#
# `-o foo` declares an "old-style" option (single-dash, multi-char) so
# fish completes `-config` rather than parsing it as five short flags.
# Go's stdlib flag package accepts both -foo and --foo; we register
# both forms so either prefix completes.

# File-completion flags.
complete -c nft-tui -o config -l config -r -F -d 'TOML config file path'
complete -c nft-tui -o audit-dir -l audit-dir -r -a '(__fish_complete_directories)' -d 'Directory for committed nft scripts'

# Value-set flags.
complete -c nft-tui -o theme -l theme -x -a 'default high-contrast mono' -d 'Colour theme'
complete -c nft-tui -o columns -l columns -x -a 'default minimal debug wide' -d 'Rule-list column preset'
complete -c nft-tui -o refresh -l refresh -x -a '250ms 500ms 1s 2s 5s 10s 30s 1m 0s' -d 'Live-counter refresh interval'

# Boolean flags.
complete -c nft-tui -o dump -l dump -d 'Print ruleset summary to stdout and exit'
complete -c nft-tui -o write -l write -d 'Enable edit/commit affordances'
complete -c nft-tui -o monitor -l monitor -d 'Subscribe to kernel netlink events for instant refresh'
complete -c nft-tui -o version -l version -d 'Print version and exit'
complete -c nft-tui -o help -l help -d 'Print help and exit'
