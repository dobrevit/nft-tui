# nft-tui — make targets for local development & manual installs.
#
# CI / release builds go through goreleaser (see .goreleaser.yaml).
# This Makefile is the thin layer for the everyday loop:
#
#   make            build the binary to ./nft-tui
#   make test       unit tests
#   make integration  integration tests inside an unshared netns
#   make install    install to PREFIX (default /usr/local)
#   make man        format the man page to stdout for proofreading

PREFIX ?= /usr/local
BINDIR := $(PREFIX)/bin
MANDIR := $(PREFIX)/share/man/man1

VERSION ?= $(shell git describe --tags --dirty --always 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X main.version=$(VERSION) \
	-X main.commit=$(COMMIT) \
	-X main.date=$(DATE)

.PHONY: build test integration vet lint precommit install uninstall man clean

# precommit runs the same set of checks that .pre-commit-config.yaml
# does, but without the pre-commit framework — for CI images, for
# anyone who doesn't have pipx/pre-commit, and as a sanity sweep
# before opening a PR.
precommit:
	./scripts/gofmt-check.sh
	go vet ./...
	@command -v golangci-lint >/dev/null 2>&1 \
		&& golangci-lint run ./... \
		|| echo 'skipped: golangci-lint not on $$PATH (install via `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest`)'
	go test -race -timeout=120s ./...

build:
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o nft-tui ./cmd/nft-tui

test:
	go test ./...

# Integration tests need real nft + CAP_NET_ADMIN; we run them inside
# `unshare -rn` which gives a fake-root user/net namespace without
# any actual privilege escalation.
integration:
	go test -tags=integration -c -o /tmp/nft-integration ./internal/nft
	unshare -rn /tmp/nft-integration -test.v -test.run=Integration

vet:
	go vet ./...

install: build
	install -D -m 0755 nft-tui $(DESTDIR)$(BINDIR)/nft-tui
	install -D -m 0644 man/nft-tui.1 $(DESTDIR)$(MANDIR)/nft-tui.1

uninstall:
	rm -f $(DESTDIR)$(BINDIR)/nft-tui
	rm -f $(DESTDIR)$(MANDIR)/nft-tui.1

man:
	groff -mandoc -Tutf8 man/nft-tui.1 | less -R

clean:
	rm -f nft-tui
	rm -f /tmp/nft-integration
