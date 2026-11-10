#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Builds release N from git and N+1 from this checkout, and runs the upgrade test
# between them (blueprint 07, rule 2). CI runs it on every change:
#
#   scripts/upgrade-check.sh [git commit of release N]
#
# Without a commit, or with one git does not know, N is the commit before HEAD.
# Needs Go, git, curl, psql and DIFFLUENCE_TEST_DATABASE_URL.
set -eu
here=$(cd "$(dirname "$0")/.." && pwd)
base=${1:-}
if [ -z "$base" ] || ! git -C "$here" cat-file -e "$base^{commit}" 2>/dev/null; then
  base=$(git -C "$here" rev-parse HEAD~1)
fi

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

echo "release N: $(git -C "$here" log -1 --format='%h %s' "$base")"
mkdir "$work/old-src"
# An archive, not a worktree: it leaves the repository as it was.
git -C "$here" archive --format=tar "$base" >"$work/old.tar"
tar -C "$work/old-src" -xf "$work/old.tar"
(cd "$work/old-src" && CGO_ENABLED=0 go build -trimpath -o "$work/old" ./cmd/diffluence)
echo "release N+1: this checkout"
(cd "$here" && CGO_ENABLED=0 go build -trimpath -o "$work/new" ./cmd/diffluence)
bash "$here/scripts/upgrade-test.sh" "$work/old" "$work/new"
