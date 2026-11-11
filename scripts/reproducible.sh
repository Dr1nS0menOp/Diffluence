#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# Does a build reproduce? Builds the web assets and the binary twice, from two copies of
# this tree in folders of a different name and depth, each with its own Go build cache and
# npm cache, and compares every file byte for byte (OPS-095, spec 057). Names each file that
# differs and exits 1 then. Needs Go and Node, as in the development toolbox:
#   scripts/dev bash scripts/reproducible.sh
set -euo pipefail

src=$(pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
# A fixed version and commit date: the same inputs a release gives both builds.
version=${VERSION:-v0.0.0-repro}
date=${BUILD_DATE:-2026-01-01}
pkg=github.com/diffluence/diffluence/internal/version

build_in() {
  local dir=$1
  mkdir -p "$dir"
  # The tree as it is, without what a build makes or downloads.
  tar -C "$src" --exclude=./.git --exclude=./bin --exclude=./dist --exclude=./build \
    --exclude=./web/node_modules --exclude=./collab/node_modules --exclude=./web/test-results \
    --exclude='./internal/webui/dist/assets' --exclude=./internal/webui/dist/index.html \
    -cf - . | tar -C "$dir" -xf -
  (
    cd "$dir/web"
    npm_config_cache="$dir/.npm" npm ci --no-fund --no-audit --loglevel=error >/dev/null
    npx vite build --logLevel error >/dev/null
  )
  (
    cd "$dir"
    GOCACHE="$dir/.gocache" GOFLAGS=-buildvcs=false CGO_ENABLED=0 \
      go build -trimpath -ldflags "-s -w -X $pkg.Version=$version -X $pkg.Date=$date" \
      -o "$dir/out/diffluence" ./cmd/diffluence
  )
}

echo "building in two places..."
build_in "$work/one/a"
build_in "$work/two/deeper/b"

differ=0
compare() {
  local one=$1 two=$2 name=$3
  if ! cmp -s "$one" "$two"; then
    echo "DIFFERS  $name"
    differ=$((differ + 1))
  fi
}
compare "$work/one/a/out/diffluence" "$work/two/deeper/b/out/diffluence" diffluence
count=1
while IFS= read -r -d '' file; do
  rel=${file#"$work/one/a/"}
  if [ ! -e "$work/two/deeper/b/$rel" ]; then
    echo "MISSING  $rel (only in the first build)"
    differ=$((differ + 1))
  else
    compare "$file" "$work/two/deeper/b/$rel" "$rel"
  fi
  count=$((count + 1))
done < <(find "$work/one/a/internal/webui/dist" -type f -print0)
extra=$(cd "$work/two/deeper/b" && find internal/webui/dist -type f | wc -l)
if [ "$extra" -ne $((count - 1)) ]; then
  echo "DIFFERS  internal/webui/dist (the second build has $extra files, the first $((count - 1)))"
  differ=$((differ + 1))
fi

if [ "$differ" -gt 0 ]; then
  echo "$differ of $count files differ: the build does not reproduce"
  exit 1
fi
echo "the build reproduces: $count files the same, sha256 of the binary $(sha256sum <"$work/one/a/out/diffluence" | cut -c1-16)"
