<!--
Thanks for the PR! release-drafter groups changes by label for the
next release notes; an autolabeler in .github/release-drafter.yml
will guess the right label from your branch and PR title, but you
can also pick one manually from:

    feature / bug / fix / docs / chore / refactor / test / ci /
    dependencies / security / breaking

Title prefix conventions (see CONTRIBUTING.md):
    Phase N.M: …      roadmap slices
    deferred: …       fixes for items in docs/07-deferred.md
    docs: …           documentation-only changes
    fix: …            bug fixes
    refactor: …       no-behaviour-change cleanups
    test: …           test-only changes
-->

## What changed

<!-- One or two sentences. Reviewers read this first. -->

## Why

<!-- Link to the issue if any. If not, the smallest version of "the
     problem this solves" you can write. Skip if obvious from the title. -->

## How to verify

<!-- A reproducer or test invocation that exercises the change.
     For UI changes, paste a brief transcript or screenshot. -->

## Checklist

- [ ] `go vet ./...` and `gofmt -l .` are clean
- [ ] `make test` passes
- [ ] For kernel-facing changes: `make integration` passes (or CI did)
- [ ] New behaviour is covered by a test
- [ ] Docs / man page / help overlay updated if user-visible
