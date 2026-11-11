#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# The weekly restore of a real installation (OPS-081, spec 054). Restores the newest
# backup into a throw-away Compose project on its own port, runs the checks of a restore
# (audit chain, keys), counts what came back, prints how long it took, and removes the
# project. It never touches the live installation. Run it from the folder with
# compose.yaml, from a timer:
#   bash scripts/restore-check.sh /path/to/backups /path/to/backup.key
# It exits non-zero when the restore fails, so the timer's failure alert reaches a person.
# Every later stage of a pipe here reads all of its input, so pipefail is safe.
set -euo pipefail
backups=${1:?usage: restore-check.sh <backups folder> <key file>}
key=$(realpath "${2:?usage: restore-check.sh <backups folder> <key file>}")
src=$(pwd)
[ -f "$src/compose.yaml" ] || { echo "run it from the folder with compose.yaml" >&2; exit 2; }
newest=$(find "$backups" -mindepth 1 -maxdepth 1 -type d -name 'diffluence-*' | sort | tail -n 1)
[ -n "$newest" ] || { echo "RESTORE CHECK FAILED: no backup in $backups" >&2; exit 1; }

# tell_console reports the result to the live installation's console (spec 058), which
# shows it and mails the administrators when it failed. It goes through the running node,
# or a one-off container when the node is down; a console that cannot be told is only
# mentioned, because the exit code of this script is what a timer alerts on.
tell_console() {
  local args=(backup restore-checked --name "$(basename "$newest")" --seconds "${1:-0}")
  [ -z "${2:-}" ] || args+=(--failed "$2")
  # The throw-away project's settings must not reach the live installation's Compose.
  if (unset COMPOSE_PROJECT_NAME DIFFLUENCE_PORT DIFFLUENCE_PUBLIC_URL DIFFLUENCE_IMAGE
    cd "$src" && { docker compose exec -T diffluence diffluence "${args[@]}" ||
    docker compose run --rm --no-deps -T diffluence "${args[@]}"; }) >/dev/null 2>&1; then
    return 0
  fi
  echo "the console could not be told the result of the restore check" >&2
}
fail() {
  tell_console "${took:-0}" "$1"
  echo "RESTORE CHECK FAILED: $1" >&2
  exit 1
}
# The image the live installation runs, used as it is: the check never builds or tags one.
live=$(docker compose ps -aq diffluence)
image=$(docker inspect --format '{{.Config.Image}}' "$live" 2>/dev/null) || image=
[ -n "$image" ] || fail "no Diffluence container here to take the image from"

work=$(mktemp -d)
project=dflrestorecheck$$
port=${DIFFLUENCE_RESTORE_CHECK_PORT:-18097}
cleanup() {
  if ! (cd "$work" && COMPOSE_PROJECT_NAME=$project docker compose down -v >/dev/null 2>&1); then
    echo "could not remove the throw-away project $project: docker compose -p $project down -v" >&2
  fi
  rm -rf "$work"
}
trap cleanup EXIT

# Only what a restore needs; the image of the live installation is used as it is.
cp "$src/compose.yaml" "$work/"
cp -r "$src/scripts" "$src/deploy" "$work/"
cd "$work"
export COMPOSE_PROJECT_NAME=$project DIFFLUENCE_PORT=$port DIFFLUENCE_PUBLIC_URL=http://localhost:$port DIFFLUENCE_IMAGE=$image

start=$(date +%s)
if ! bash scripts/restore.sh "$newest" "$key" >restore.log 2>&1; then
  cat restore.log >&2
  took=$(($(date +%s) - start))
  fail "$(basename "$newest") did not restore"
fi
took=$(($(date +%s) - start))
counts=$(docker compose exec -T postgres psql -U diffluence_owner -d diffluence -tA -F ' ' -c \
  "SELECT (SELECT count(*) FROM pages), (SELECT count(*) FROM attachments), (SELECT count(*) FROM users)")
read -r pages files people <<<"$counts"
if [ "${people:-0}" -lt 1 ]; then
  fail "$(basename "$newest") restored without any person"
fi
tell_console "$took"
echo "restore check passed in ${took}s: $(basename "$newest"), $pages pages, $files files, $people people, audit chain and keys checked"
