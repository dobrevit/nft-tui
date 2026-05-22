#!/usr/bin/env bash
# gofmt-check.sh — recursive gofmt check used by pre-commit and the
# Makefile. `gofmt -l .` only inspects the top-level directory, so
# subpackage drift is invisible to it. Enumerate every .go file
# explicitly and hand them to gofmt.
#
# On a clean tree: exits 0 silently.
# On drift: runs `gofmt -w` over the offending files, prints what
#           changed, and exits 1 so pre-commit aborts the commit.
#           The contributor re-stages (`git add`) and tries again.

set -euo pipefail

# Collect every Go file outside the VCS metadata.
mapfile -t files < <(find . -type f -name '*.go' -not -path './.git/*')

if [ ${#files[@]} -eq 0 ]; then
    exit 0
fi

# `gofmt -l` lists files that need formatting (no output on a clean tree).
needs_fmt=$(gofmt -l "${files[@]}")

if [ -z "$needs_fmt" ]; then
    exit 0
fi

echo "::: gofmt would change the following files:"
printf '  %s\n' $needs_fmt
echo "::: applying gofmt -w; please re-stage (git add) and commit again."

# shellcheck disable=SC2086  # we WANT word-splitting on $needs_fmt
gofmt -w $needs_fmt

exit 1
