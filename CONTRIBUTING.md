# Contributing to nft-tui

Thanks for considering a contribution. Contributions of every size are
welcome — typo fixes, new renderer cases, bug reports, feature ideas.
This document is the short version of "how to get a change in."

For anything bigger than a typo, please [open an issue][issues] first
so we can agree on the shape of the change before you spend time on
the code. For security-sensitive issues see
[Reporting a vulnerability](#reporting-a-vulnerability) below.

[issues]: https://github.com/dobrevit/nft-tui/issues

## Setting up

You'll need:

- **Go ≥ 1.22** (the repo currently builds on 1.26.2)
- **`nft`** (nftables userspace) on `$PATH` — required at runtime and
  for integration tests
- **An unshared user/net namespace tool** (`unshare -rn`) if you want
  to run the integration tests without root

A typical loop:

```sh
git clone https://github.com/dobrevit/nft-tui
cd nft-tui
make build         # → ./nft-tui (static, CGO disabled)
make test          # unit tests (no kernel needed)
make integration   # integration tests inside unshare -rn
make precommit     # gofmt + go vet + golangci-lint + go test
```

### Pre-commit hooks (recommended)

The repo ships a `.pre-commit-config.yaml` so [pre-commit](https://pre-commit.com)
runs gofmt, go vet, golangci-lint, and a few hygiene checks on every
commit (and `go test` on every push). One-time setup:

```sh
pipx install pre-commit   # or: pip install --user pre-commit
pre-commit install                          # commit-time hooks
pre-commit install --hook-type pre-push     # push-time go test
```

After that, `git commit` runs the hooks. If gofmt auto-formatted
anything, the commit aborts with a clear message — `git add` the
fixups and try again.

Don't want the pre-commit framework? `make precommit` runs the same
checks without it.

If `make integration` fails with `operation not permitted`, your
distro doesn't allow unprivileged user namespaces. Either enable
them (`sysctl kernel.unprivileged_userns_clone=1`) or run the tests
as root inside a throwaway VM.

## What to work on

The roadmap in [docs/06-roadmap.md](docs/06-roadmap.md) is shipped;
the open items live in [docs/07-deferred.md](docs/07-deferred.md).
The "Open v1.1+ ideas" section there is the friendliest pickup list:

- **nft text → form fields parser** so the form editor can prefill
  on Edit (modeEdit currently defaults to raw mode)
- **Theme-driven dynamic-colour tags** — plumb the semantic
  `Accent`/`Good`/`Bad`/`Warning` colours through render helpers
- **Per-host config file** (`~/.config/nft-tui/config.toml`) for
  default theme / columns / refresh / audit dir

Smaller good-first-issues:

- More renderer cases — anything that surfaces as `<expr:T>` in
  `nft-tui -dump` is fair game. See `internal/nft/render.go`.
- Per-distro packaging niceties (e.g., shell completions; Phase 6.2
  produces the binary + man page but nothing else).
- Test coverage for less-exercised code paths in `internal/ui/`.

## Code conventions

- **`gofmt`** is mandatory. The pre-commit hook (or `make precommit`)
  runs it for you; CI rejects unformatted code.
- **`go vet ./...`** must be clean.
- **No new packages without a reason**. The existing layout is
  `internal/model`, `internal/nft` (read + write adapter),
  `internal/staged` (staged ops), `internal/ui` (everything tview).
- **Tests** for new logic. Renderer changes need a case in
  `internal/nft/render_test.go`. UI logic that doesn't depend on
  tview should be testable in isolation (see the existing
  `clipboard_test.go`, `search_test.go`, `spark_test.go`).
- **Integration tests** for changes to the read/write/commit/restore
  path; tag with `//go:build integration` and run them inside
  `unshare -rn` to avoid touching the host kernel.
- **No new dependencies** without a strong case. The current pin set
  (`google/nftables`, `tview` + `tcell`, stdlib) is deliberate.

## Commit message style

The repo uses one-line subjects describing the *what*, followed by a
body that explains *why* / what's tricky / how to verify. Examples
from the history:

```
Phase 4.2: per-rule sparkline in the monitor
deferred: o / O insert keys, InsertRule.After
docs: mark v1.0 ready — sync all docs with shipped state
```

Subject prefixes:

- `Phase N.M:` for roadmap slices
- `deferred:` for fixes to items in `docs/07-deferred.md`
- `docs:` for documentation-only changes
- `fix:` / `refactor:` / `test:` for smaller, focused changes

Keep subjects under ~70 chars; wrap the body at 78. No
`Co-Authored-By:` trailers, please.

## Submitting a change

1. Fork the repo, branch off `main`.
2. Make your change. `make test` and (for kernel-facing changes)
   `make integration` should pass.
3. Open a PR. In the description, link to the issue (if any) and
   describe the why. If you added a new feature, paste a short
   transcript or screenshot from the TUI.
4. Be ready to iterate — review feedback is meant in good faith.

By submitting a contribution you agree that your work will be
licensed under the project's [MIT License](LICENSE).

## Reporting a vulnerability

If you find a security issue (a way to bypass the dry-run gate,
sandbox-escape via crafted nft input, etc.) please **don't** open a
public issue. Email **devops@dobrev.it** instead, with:

- A description of the issue and its impact.
- Reproduction steps (a minimal nft script or operator-action sequence
  is ideal).
- Your preferred attribution for the eventual disclosure.

We'll acknowledge within five working days and aim to ship a fix
before any public disclosure.

## Code of conduct

Be kind. Disagree about technical decisions, not about people.
Reviewers and contributors with maintainer hats commit to giving
feedback in good faith and to keeping the discussion on the merits.
Repeated personal attacks, harassment, or doxxing earn a ban.

## Contact

- General questions / discussion: open an issue.
- Security: `devops@dobrev.it`.
