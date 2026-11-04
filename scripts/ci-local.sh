#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
# Runs the CI steps in a fresh clone of what is committed, because a working tree
# hides problems: build output and untracked files make things pass that fail for
# everyone else. Commit first, then run in the toolbox:
#   scripts/dev sh scripts/ci-local.sh
set -eu
git config --global --add safe.directory '*'
dir=$(mktemp -d)
trap 'rm -rf "$dir"' EXIT
git clone -q "$(git rev-parse --show-toplevel)" "$dir/repo"
cd "$dir/repo"
echo "clean clone of $(git rev-parse --short HEAD)"

echo "--- go job"
make fmt-check vet staticcheck vuln test-race build

echo "--- web job"
(cd web && npm ci --no-fund --no-audit)
make web-check
(cd web && npm audit --omit=dev --audit-level=high)
make licenses

echo "--- a frontend build must not touch tracked files"
(cd web && npx vite build >/dev/null)
git status --short | grep -v '^??' && exit 1
test -f internal/webui/dist/.placeholder

echo "ci-local passed"
