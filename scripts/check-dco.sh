#!/usr/bin/env sh
# SPDX-License-Identifier: Apache-2.0
# Fails when a commit in the range lacks a DCO "Signed-off-by" line that matches its author.
# Usage: scripts/check-dco.sh <base-ref> <head-ref>
set -eu

base="${1:?base ref required}"
head="${2:?head ref required}"
fail=0

for sha in $(git rev-list --no-merges "${base}..${head}"); do
  author="$(git log -1 --format='%an <%ae>' "$sha")"
  if ! git log -1 --format='%B' "$sha" | grep -qF "Signed-off-by: ${author}"; then
    echo "::error::commit ${sha} has no 'Signed-off-by: ${author}' (use git commit -s)"
    fail=1
  fi
done

if [ "$fail" -ne 0 ]; then
  echo "DCO check failed. Fix with: git rebase --signoff ${base}"
  exit 1
fi
echo "DCO check passed."
