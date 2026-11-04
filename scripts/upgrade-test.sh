#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# The upgrade promise, tested (blueprint 07): the old version serves traffic, the
# new version starts on the same database and migrates it, both run side by side,
# and the old one stops. Not one request may fail.
#
#   scripts/upgrade-test.sh <old binary> <new binary>
#
# Needs DIFFLUENCE_TEST_DATABASE_URL pointing at a role that may create databases.
set -euo pipefail
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

echo "2. the new version starts on the same database and migrates it"
serve "$new" 8092 "$work/new" "$work/new.log"
printf '8091\n8092\n' >"$work/ports"
sleep 3

echo "3. both versions serve side by side"
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

echo "5. the audit chain written by both versions is intact"
DIFFLUENCE_DATA_DIR="$work/new" "$new" audit verify
echo "upgrade test passed"
