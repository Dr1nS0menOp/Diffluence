#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# The upgrade promise, tested (blueprint 07): the old version serves traffic, the
# new version starts on the same database and migrates it, both run side by side,
# and the old one stops. Not one request may fail. Then the old version starts
# again on the migrated database, as a rollback does.
#
#   scripts/upgrade-test.sh <old binary> <new binary>
#
# Needs DIFFLUENCE_TEST_DATABASE_URL pointing at a role that may create databases.
set -euo pipefail
# A failure inside $(...) stops the test too, not only one outside it.
shopt -s inherit_errexit
old=$1
new=$2
: "${DIFFLUENCE_TEST_DATABASE_URL:?needs a test database}"

work=$(mktemp -d)
db="dfl_upgrade_$$"
base=${DIFFLUENCE_TEST_DATABASE_URL%%\?*}
base=${base%/*}
url="$base/$db?sslmode=disable"
pids=()
stop_all() {
  for p in "${pids[@]:-}"; do kill "$p" 2>/dev/null || true; done
  wait 2>/dev/null || true
  psql "$DIFFLUENCE_TEST_DATABASE_URL" -q -c "DROP DATABASE IF EXISTS $db WITH (FORCE)" || true
  rm -rf "$work"
}
trap stop_all EXIT
psql "$DIFFLUENCE_TEST_DATABASE_URL" -q -c "CREATE DATABASE $db"

export DIFFLUENCE_DATABASE_URL=$url DIFFLUENCE_PUBLIC_URL=http://localhost:8091 DIFFLUENCE_LOG_LEVEL=warn DIFFLUENCE_SHUTDOWN_DELAY=1s

serve() { # binary port data-dir log
  DIFFLUENCE_HTTP_ADDR=":$2" DIFFLUENCE_DATA_DIR="$3" "$1" serve >"$4" 2>&1 &
  pids+=($!)
  for _ in $(seq 1 100); do
    curl -sf "http://localhost:$2/readyz" >/dev/null && return 0
    sleep 0.1
  done
  echo "server on port $2 did not get ready:"
  cat "$4"
  exit 1
}

echo "1. the old version starts and takes traffic"
echo 'correct horse battery staple' | DIFFLUENCE_DATA_DIR="$work/old" "$old" user create-admin --email admin@example.com --name Admin >/dev/null
serve "$old" 8091 "$work/old" "$work/old.log"
old_pid=${pids[-1]}

cookie="$work/jar"
login() { curl -s -o /dev/null -w '%{http_code}' -c "$cookie" -H 'Content-Type: application/json' \
  -d '{"email":"admin@example.com","password":"correct horse battery staple"}' "http://localhost:$1/api/v1/auth/login"; }
[ "$(login 8091)" = 200 ]

# Traffic: reads and writes, as a colleague would cause. The load balancer here
# is a file naming the ports that are in rotation.
echo 8091 >"$work/ports"
traffic() {
  local i=0 status port
  while [ ! -e "$work/stop" ]; do
    i=$((i + 1))
    mapfile -t ports <"$work/ports"
    port=${ports[$((i % ${#ports[@]}))]}
    for path in /api/v1/me /api/v1/admin/audit-events?limit=5 /readyz; do
      status=$(curl -s -m 10 -o /dev/null -w '%{http_code}' -b "$cookie" "http://localhost:$port$path" || echo 000)
      echo "$port $path $status" >>"$work/requests"
    done
    # A login writes a session, an audit event and an outbox event. Not every
    # round: older releases count each login against the limit per address.
    if [ $((i % 4)) = 0 ]; then
      status=$(curl -s -m 10 -o /dev/null -w '%{http_code}' -H 'Content-Type: application/json' \
        -d '{"email":"admin@example.com","password":"correct horse battery staple"}' "http://localhost:$port/api/v1/auth/login" || echo 000)
      echo "$port login $status" >>"$work/requests"
    fi
    sleep 0.05
  done
}
traffic &
traffic_pid=$!
sleep 2

# Writes cross versions too (OPS-071): what one release stores, the other must read. Most
# upgrade failures come through stored data and messages, not through reads (spec 053).
call() { # port method path [json body]: prints the answer, fails unless it is 2xx
  local out code body=()
  [ $# -ge 4 ] && body=(-d "$4")
  out=$(curl -s -m 20 -b "$cookie" -X "$2" -H 'Content-Type: application/json' "${body[@]}" \
    -w '\n%{http_code}' "http://localhost:$1$3") || out=$'\n000'
  code=${out##*$'\n'}
  if [ "${code:0:1}" != 2 ]; then
    echo "$2 $3 on port $1 answered $code: ${out%$'\n'*}" >&2
    return 1
  fi
  printf '%s' "${out%$'\n'*}"
}
field() { grep -o "\"$1\":\"[^\"]*\"" | head -1 | cut -d'"' -f4; }
doc() { printf '{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"%s"}]}]}' "$1"; }
contains() { # text what: fails with both when the text lacks it
  case $1 in *"$2"*) return 0 ;; esac
  echo "expected \"$2\" in: ${1:0:300}" >&2
  return 1
}
write_on() { # port tag: a page published, a draft, a comment and a file; prints the page
  local page file
  page=$(call "$1" POST /api/v1/pages "{\"space_id\":\"$space\",\"parent_id\":null,\"title\":\"Written by $2\"}" | field id)
  [ -n "$page" ] || return 1
  call "$1" POST "/api/v1/pages/$page/publish" "{\"title\":\"Written by $2\",\"document\":$(doc "published by $2"),\"base_version\":0}" >/dev/null
  call "$1" PUT "/api/v1/pages/$page/draft" "{\"title\":\"Written by $2\",\"document\":$(doc "draft by $2"),\"base_version\":1}" >/dev/null
  call "$1" POST "/api/v1/pages/$page/comments" "{\"body\":$(doc "comment by $2")}" >/dev/null
  file=$(curl -s -m 20 -b "$cookie" -H 'Content-Type: application/octet-stream' --data-binary "file by $2" \
    "http://localhost:$1/api/v1/pages/$page/attachments?filename=$2.txt" | field id)
  [ -n "$file" ] || { echo "the upload on port $1 failed" >&2; return 1; }
  echo "$page"
}
read_on() { # port page tag: everything write_on stored, read through another release
  local file
  contains "$(call "$1" GET "/api/v1/pages/$2")" "published by $3"
  contains "$(call "$1" GET "/api/v1/pages/$2/draft")" "draft by $3"
  contains "$(call "$1" GET "/api/v1/pages/$2/comments")" "comment by $3"
  file=$(call "$1" GET "/api/v1/pages/$2/attachments" | field id)
  [ "$(call "$1" GET "/api/v1/attachments/$file/content")" = "file by $3" ] ||
    { echo "the file written by $3 reads differently on port $1" >&2; return 1; }
}
space=$(call 8091 POST /api/v1/spaces '{"key":"UPGRADE","name":"Written across versions"}' | field id)
by_old=$(write_on 8091 old)

newest() { psql "$url" -tAc 'SELECT max(version) FROM schema_migrations'; }
before=$(newest)

# As an operator does before an upgrade, a backup is noted; an upgrade asks for one of at
# most a day (ADR-040). A release from before "backup note" existed cannot note one, and
# the new binary does it then.
DIFFLUENCE_DATA_DIR="$work/old" "$old" backup note >/dev/null 2>&1 ||
  DIFFLUENCE_DATA_DIR="$work/old" "$new" backup note >/dev/null

echo "2. the new version starts on the same database and migrates it"
# Every node of an installation has the same key (DIFFLUENCE_MASTER_KEY_FILE) and the same
# file store; each keeps its own queue folder.
mkdir -p "$work/new" "$work/old/objects"
cp "$work/old/master.key" "$work/new/master.key"
ln -s "$work/old/objects" "$work/new/objects"
serve "$new" 8092 "$work/new" "$work/new.log"
printf '8091\n8092\n' >"$work/ports"
# A real upgrade leaves one marker in the audit log before it migrates (OPS-041).
after=$(newest)
markers=$(psql "$url" -tAc "SELECT count(*) FROM audit_events WHERE action = 'system.upgrade_started'")
echo "migrations $before to $after, upgrade markers: $markers"
if [ "$after" != "$before" ] && [ "$markers" != 1 ]; then
  echo "the upgrade left $markers markers, not one"
  exit 1
fi
sleep 3

echo "3. both versions serve side by side, and each reads what the other wrote"
read_on 8092 "$by_old" old
by_new=$(write_on 8092 new)
read_on 8091 "$by_new" new
sleep 3

echo "4. the load balancer drops the old node, which then stops gracefully"
echo 8092 >"$work/ports"
sleep 1
kill -TERM "$old_pid"
wait "$old_pid" || true
sleep 3

touch "$work/stop"
wait "$traffic_pid"

total=$(wc -l <"$work/requests")
failed=$(grep -vc ' 200$' "$work/requests" || true)
on_old=$(grep -c '^8091 ' "$work/requests" || true)
on_new=$(grep -c '^8092 ' "$work/requests" || true)
echo "requests: $total (old $on_old, new $on_new), failed: $failed"
if [ "$failed" -gt 0 ]; then
  grep -v ' 200$' "$work/requests" | sort | uniq -c | sort -rn | head
  echo "--- old log"; tail -5 "$work/old.log"
  echo "--- new log"; tail -5 "$work/new.log"
  exit 1
fi
[ "$on_old" -gt 0 ] && [ "$on_new" -gt 0 ]

echo "5. rolling back: the old version starts again on the migrated database"
# The new node stops first, as in a rollback; the old one keeps its own queue folder.
for p in "${pids[@]}"; do kill -TERM "$p" 2>/dev/null || true; done
wait 2>/dev/null || true
pids=()
serve "$old" 8091 "$work/old" "$work/rollback.log"
status=$(login 8091)
if [ "$status" = 200 ]; then
  status=$(curl -s -o /dev/null -w '%{http_code}' -b "$cookie" http://localhost:8091/api/v1/me)
fi
if [ "$status" != 200 ]; then
  echo "after the rollback, a login and a read answered $status"
  tail -5 "$work/rollback.log"
  exit 1
fi
read_on 8091 "$by_old" old
read_on 8091 "$by_new" new

# The events and jobs both releases left behind went through: none failed, none set aside,
# and the outbox empties (OPS-070).
for _ in $(seq 1 60); do
  [ "$(psql "$url" -tAc 'SELECT count(*) FROM outbox_events WHERE published_at IS NULL')" = 0 ] && break
  sleep 0.5
done
left=$(psql "$url" -tAc "SELECT
  (SELECT count(*) FROM outbox_events WHERE published_at IS NULL) || ' unsent events, ' ||
  (SELECT count(*) FROM event_failures) || ' events set aside, ' ||
  (SELECT count(*) FROM jobs WHERE status = 'failed') || ' failed jobs'")
echo "after both releases: $left"
if [ "$left" != "0 unsent events, 0 events set aside, 0 failed jobs" ]; then
  tail -5 "$work/rollback.log"
  exit 1
fi
kill -TERM "${pids[-1]}"
wait "${pids[-1]}" || true
pids=()

echo "6. the audit chain written by both versions is intact"
DIFFLUENCE_DATA_DIR="$work/new" "$new" audit verify
echo "upgrade test passed"
