# Examples

Sample nftables rulesets that load cleanly via `nft -f`, and a
sample `nft-tui` config.

## Rulesets

| File | What it models |
|---|---|
| [`edge-router.nft`](edge-router.nft) | Small office / home edge router. Two interfaces (WAN/LAN), masquerade outbound, port forward for an inside web server, conservative WAN-side input filter. |
| [`web-server.nft`](web-server.nft) | Single-host server. SSH (allowlisted + ratelimited), HTTP+HTTPS to the world, ICMP essentials, deny everything else. |
| [`container-host.nft`](container-host.nft) | A host running Docker / podman alongside its own filter. Keeps our rules in a separate `inet filter` table so the runtime's chains coexist. |

Each one runs through `nft -c` clean. They aren't production-ready
templates — adjust interface names, allowlist sets, and any
listed ports to match your actual environment.

## Trying one with nft-tui

To play with the UI against an example without touching your host's
real ruleset, load it inside an unprivileged user/network namespace:

```sh
unshare -rn sh -c 'nft -f examples/web-server.nft && ./nft-tui'
```

`unshare -rn` (linux ≥ 3.8) gives the inner shell `CAP_NET_ADMIN`
inside a brand-new netns, so the ruleset lives there and disappears
when you exit. Same trick `make integration` uses for kernel-facing
tests.

## Config file

[`config.toml`](config.toml) is a fully-commented sample for
`~/.config/nft-tui/config.toml` (or `$XDG_CONFIG_HOME/nft-tui/`).
Every setting is optional; uncomment what you want pinned. CLI
flags always override the file.
