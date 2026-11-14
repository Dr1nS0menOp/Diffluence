#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Every Go test under the race detector, as "make test-race" runs them. internal/app holds most
# of the tests, and one process runs them one after another, so its tests are dealt over
# TEST_SHARDS processes (default 4) that run beside the other packages. Each test the package
# lists runs exactly once, in one shard; the shards together are checked against that list.
# Every test creates its own database, so the processes share nothing but the server.
set -eu
shards=${TEST_SHARDS:-4}
case "$shards" in
  '' | *[!0-9]* | 0) echo "TEST_SHARDS must be a whole number above zero, not $shards" >&2; exit 2 ;;
esac
app=./internal/app
flags=(-race -count=1 -timeout=45m)
out=$(mktemp -d)
trap 'rm -rf "$out"' EXIT

# The tests, examples and fuzz targets of internal/app, one name per line. "go test -list"
# compiles the package as the shards will, so they reuse the build.
if ! go test "${flags[@]}" -list '.*' "$app" >"$out/listed" 2>&1; then
  cat "$out/listed"
  exit 1
fi
if ! grep -E '^(Test|Example|Fuzz)' "$out/listed" >"$out/unsorted"; then
  echo "internal/app lists no tests" >&2
  exit 1
fi
sort "$out/unsorted" >"$out/names"
awk -v n="$shards" -v dir="$out" '{ print > (dir "/shard." ((NR - 1) % n)) }' "$out/names"
# A shard regexp names every test of the shard in full, so it matches nothing else.
cat "$out"/shard.* | sort | cmp -s - "$out/names" || {
  echo "the shards of internal/app do not hold exactly its tests" >&2
  exit 1
}

app_pkg=$(go list "$app")
go list ./... | grep -vxF "$app_pkg" >"$out/others"
pids=()
# shellcheck disable=SC2046 # one package path per word, as go list prints them
go test "${flags[@]}" $(cat "$out/others") >"$out/result.others" 2>&1 &
pids+=("$!")
for shard in "$out"/shard.*; do
  go test "${flags[@]}" -run "^($(paste -sd '|' "$shard"))\$" "$app" >"$out/result.app.${shard##*.}" 2>&1 &
  pids+=("$!")
done
status=0
for pid in "${pids[@]}"; do
  wait "$pid" || status=1
done
cat "$out/result.others"
for shard in "$out"/result.app.*; do
  echo "internal/app, shard ${shard##*.} of $shards:"
  cat "$shard"
done
exit "$status"
