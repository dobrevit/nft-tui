# Container image for nft-tui.
#
# Built by goreleaser at release time (see .goreleaser.yaml's
# `dockers:` block). goreleaser places the prebuilt binary in the
# build context before invoking `docker build`, so the COPY below
# picks it up — there's no `go build` step here.
#
# Run on a host to manage its real nftables ruleset:
#
#     docker run --rm -it --net=host --cap-add=NET_ADMIN \
#       ghcr.io/dobrevit/nft-tui:latest
#
# `--net=host` is required (the netlink socket lives on the host's
# net namespace) and `--cap-add=NET_ADMIN` is what lets the
# `nftables` userspace inside the container talk to nf_tables on
# the host. The binary is statically linked (CGO_ENABLED=0) so the
# Alpine base provides only the `nft` userspace tool used for
# commit and restore.

FROM alpine:3.22

# nftables ships the `nft` CLI; ca-certificates makes outbound TLS
# work if the operator ever curls a snapshot from somewhere; tini
# gives us correct signal handling for an interactive TUI process.
RUN apk add --no-cache nftables ca-certificates tini

COPY nft-tui                  /usr/bin/nft-tui
COPY examples/config.toml     /usr/share/doc/nft-tui/config.toml.example
COPY man/nft-tui.1            /usr/share/man/man1/nft-tui.1

# A non-root user is conventional for container images, but
# nft-tui needs CAP_NET_ADMIN — the operator passes --cap-add at
# runtime. Running as root inside the (host-net) container is the
# expected mode; no USER directive.

ENTRYPOINT ["/sbin/tini", "--", "/usr/bin/nft-tui"]
