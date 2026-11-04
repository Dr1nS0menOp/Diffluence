#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# The restore drill (blueprint 07): install, add content, back up, wipe everything,
# restore on a clean installation, and prove that all of it came back. Uses its own
# compose projects and port, and removes them afterwards.
set -euo pipefail
src=$(cd "$(dirname "$0")/.." && pwd)
work=$(mktemp -d)
port=18095
export DIFFLUENCE_PORT=$port DIFFLUENCE_PUBLIC_URL=http://localhost:$port
url=http://localhost:$port

fresh() { # a clean copy of the installation files
  mkdir -p "$work/$1"
  tar -C "$src" --exclude=./web/node_modules --exclude=./.git --exclude=./secrets --exclude=./web/test-results -cf - . | tar -C "$work/$1" -xf -
}
cleanup() {
  for p in first second; do
    [ -d "$work/$p" ] && (cd "$work/$p" && COMPOSE_PROJECT_NAME=dfldrill$p docker compose down -v >/dev/null 2>&1 || true)
  done
  docker run --rm -v "$work:/w" busybox rm -rf /w/backups >/dev/null 2>&1 || true
  rm -rf "$work"
}
trap cleanup EXIT
jar=$work/jar
api() { curl -sf -b "$jar" -c "$jar" -H 'Content-Type: application/json' "$@"; }
first_id() { grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4; }

echo "1. a real installation with content"
fresh first
cd "$work/first"
export COMPOSE_PROJECT_NAME=dfldrillfirst
sh scripts/setup-secrets.sh >/dev/null
docker compose up -d --build --wait --wait-timeout 120 >/dev/null 2>&1
printf '%s\n' 'correct horse battery staple' | docker compose exec -T diffluence diffluence user create-admin --email admin@example.com --name Admin >/dev/null
api -d '{"email":"admin@example.com","password":"correct horse battery staple"}' "$url/api/v1/auth/login" >/dev/null
space=$(api -d '{"key":"ENG","name":"Engineering"}' "$url/api/v1/spaces" | first_id)
page=$(api -d "{\"space_id\":\"$space\",\"title\":\"Important\"}" "$url/api/v1/pages" | first_id)
doc='{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"the words that must survive"}]}]}'
api -d "{\"title\":\"Important\",\"base_version\":0,\"document\":$doc}" "$url/api/v1/pages/$page/publish" >/dev/null
echo 'attached file contents' >"$work/note.txt"
curl -sf -b "$jar" -H 'Content-Type: application/octet-stream' --data-binary "@$work/note.txt" "$url/api/v1/pages/$page/attachments?filename=note.txt" >"$work/att.json"
attachment=$(first_id <"$work/att.json")
before=$(docker compose exec -T postgres psql -U diffluence_owner -d diffluence -tAc 'SELECT count(*) FROM audit_events')

echo "2. back up, then destroy the whole installation"
start=$(date +%s)
sh scripts/backup.sh "$work/backups" >/dev/null
docker compose down -v >/dev/null 2>&1

echo "3. restore on a clean installation"
fresh second
cd "$work/second"
export COMPOSE_PROJECT_NAME=dfldrillsecond
sh scripts/restore.sh "$work"/backups/diffluence-* | tail -2
took=$(($(date +%s) - start))

echo "4. everything came back"
rm -f "$jar"
api -d '{"email":"admin@example.com","password":"correct horse battery staple"}' "$url/api/v1/auth/login" >/dev/null
api "$url/api/v1/pages/$page" | grep -q 'the words that must survive'
[ "$(curl -sf -b "$jar" "$url/api/v1/attachments/$attachment/content?download=1")" = "attached file contents" ]
api "$url/api/v1/search?q=survive" | grep -q '"title":"Important"'
after=$(docker compose exec -T postgres psql -U diffluence_owner -d diffluence -tAc 'SELECT count(*) FROM audit_events')
[ "$after" -ge "$before" ]
# The application role must still be unable to touch the audit log after a restore.
password=$(cat secrets/db_app_password.txt)
if docker compose exec -T -e PGPASSWORD="$password" postgres psql -h localhost -U diffluence_app -d diffluence -c "DELETE FROM audit_events" >/dev/null 2>&1; then
  echo "the application role can delete audit events after a restore"
  exit 1
fi
echo "restore drill passed in ${took}s: page, attachment, search, login, audit chain and role separation"
