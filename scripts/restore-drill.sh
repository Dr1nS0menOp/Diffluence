#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# The restore drill (blueprint 07): install, add content, make an encrypted backup, wipe everything,
# restore on a clean installation, and prove that all of it came back. Uses its own
# compose projects and port, and removes them afterwards.
set -euo pipefail
src=$(cd "$(dirname "$0")/.." && pwd)
work=$(mktemp -d)
port=18095
export DIFFLUENCE_PORT=$port DIFFLUENCE_PUBLIC_URL=http://localhost:$port
# Images of its own: diffluence:local is the image a real installation on this Docker runs,
# and the drill must never replace it.
export DIFFLUENCE_IMAGE=diffluence:drill-$$ DIFFLUENCE_BACKUP_IMAGE=diffluence-backup:drill-$$
url=http://localhost:$port

fresh() { # a clean copy of the installation files
  mkdir -p "$work/$1"
  tar -C "$src" --exclude=./web/node_modules --exclude=./.git --exclude=./secrets --exclude=./backups \
    --exclude=./backup-key --exclude=./web/test-results -cf - . | tar -C "$work/$1" -xf -
}
cleanup() {
  for p in first second; do
    [ -d "$work/$p" ] && (cd "$work/$p" && COMPOSE_PROJECT_NAME=dfldrill$p docker compose down -v --rmi local >/dev/null 2>&1 || true)
  done
  # The backup service writes as root.
  docker run --rm -v "$work:/w" busybox rm -rf /w/backups /w/first/backups >/dev/null 2>&1 || true
  rm -rf "$work"
  docker rmi -f "$DIFFLUENCE_IMAGE" "$DIFFLUENCE_BACKUP_IMAGE" >/dev/null 2>&1 || true
}
noted() {
  docker compose exec -T postgres psql -U diffluence_owner -d diffluence -tAc "SELECT count(*) FROM audit_events WHERE action = 'system.backup_noted'"
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
# Without the backup service: the drill counts the backups it makes itself, then runs the
# service once in step 2c.
docker compose up -d --build --wait --wait-timeout 120 postgres diffluence >/dev/null 2>&1
printf '%s\n' 'correct horse battery staple' | docker compose exec -T diffluence diffluence user create-admin --email admin@example.com --name Admin >/dev/null
api -d '{"email":"admin@example.com","password":"correct horse battery staple"}' "$url/api/v1/auth/login" >/dev/null
space=$(api -d '{"key":"ENG","name":"Engineering"}' "$url/api/v1/spaces" | first_id)
page=$(api -d "{\"space_id\":\"$space\",\"title\":\"Important\"}" "$url/api/v1/pages" | first_id)
doc='{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"the words that must survive"}]}]}'
api -d "{\"title\":\"Important\",\"base_version\":0,\"document\":$doc}" "$url/api/v1/pages/$page/publish" >/dev/null
echo 'attached file contents' >"$work/note.txt"
curl -sf -b "$jar" -H 'Content-Type: application/octet-stream' --data-binary "@$work/note.txt" "$url/api/v1/pages/$page/attachments?filename=note.txt" >"$work/att.json"
attachment=$(first_id <"$work/att.json")

echo "1b. start replacing master.key, and back up halfway (spec 034)"
# A restart after each step loads the keys, as an operator does on every node.
docker compose exec -T diffluence diffluence key add >/dev/null
docker compose up -d --force-recreate --wait diffluence >/dev/null 2>&1
docker compose exec -T diffluence diffluence key promote >/dev/null
docker compose up -d --force-recreate --wait diffluence >/dev/null 2>&1
before=$(docker compose exec -T postgres psql -U diffluence_owner -d diffluence -tAc 'SELECT count(*) FROM audit_events')

echo "2. back up, then destroy the whole installation"
start=$(date +%s)
head -c 32 /dev/urandom | base64 >"$work/backup.key"
if bash scripts/backup.sh "$work/backups" "$work/no-such.key" >/dev/null 2>&1; then
  echo "a backup was made without a key"
  exit 1
fi
mkdir -p "$work/backups"
cp "$work/backup.key" "$work/backups/backup.key"
if bash scripts/backup.sh "$work/backups" "$work/backups/backup.key" >/dev/null 2>&1; then
  echo "a backup was made with its key beside it"
  exit 1
fi
rm "$work/backups/backup.key"
bash scripts/backup.sh "$work/backups" "$work/backup.key" >/dev/null
if ls "$work"/backups/diffluence-*/ | grep -qv -e '\.gpg$' -e '^SHA256SUMS$'; then
  echo "something in the backup is not encrypted"
  exit 1
fi
# Only the backup that was made is noted, for the checks before an upgrade.
if [ "$(noted)" != 1 ]; then
  echo "the backup was noted $(noted) times, not once"
  exit 1
fi

echo "2b. back up while the node is down, as an installation stuck before an upgrade is"
docker compose stop diffluence >/dev/null 2>&1
sleep 1
bash scripts/backup.sh "$work/backups" "$work/backup.key" >/dev/null
if [ "$(noted)" != 2 ]; then
  echo "a backup made while the node was down was not noted"
  exit 1
fi
docker compose up -d --wait diffluence >/dev/null 2>&1
for b in "$work"/backups/diffluence-*; do
  (cd "$b" && sha256sum --quiet -c SHA256SUMS)
done

echo "2c. the backup service of compose.yaml, once"
if DIFFLUENCE_BACKUP_KEY_DIR="$work/no-key" docker compose run --rm backup once >/dev/null 2>&1; then
  echo "the backup service made a backup without a key"
  exit 1
fi
docker compose run --rm backup once | tail -1
if [ "$(noted)" != 3 ]; then
  echo "the backup of the service was not noted"
  exit 1
fi
service_backup=$(find backups -mindepth 1 -maxdepth 1 -type d -name 'diffluence-*' | head -n 1)
if [ -z "$service_backup" ] || find backups -mindepth 1 -maxdepth 1 -name '.partial-*' | grep -q .; then
  echo "the backup service left no complete backup"
  exit 1
fi
if ls "$service_backup" | grep -qv -e '\.gpg$' -e '^SHA256SUMS$'; then
  echo "something in the service's backup is not encrypted"
  exit 1
fi
service_backup=$(realpath "$service_backup")
service_key=$(realpath backup-key/backup.key)
docker compose down -v >/dev/null 2>&1

echo "3. restore the service's backup on a clean installation"
fresh second
cd "$work/second"
export COMPOSE_PROJECT_NAME=dfldrillsecond
head -c 32 /dev/urandom | base64 >"$work/wrong.key"
if bash scripts/restore.sh "$service_backup" "$work/wrong.key" >/dev/null 2>&1; then
  echo "a wrong key opened the backup"
  exit 1
fi
bash scripts/restore.sh "$service_backup" "$service_key" | tail -2
took=$(($(date +%s) - start))

echo "4. everything came back"
rm -f "$jar"
api -d '{"email":"admin@example.com","password":"correct horse battery staple"}' "$url/api/v1/auth/login" >/dev/null
api "$url/api/v1/pages/$page" | grep -q 'the words that must survive'
[ "$(curl -sf -b "$jar" "$url/api/v1/attachments/$attachment/content?download=1")" = "attached file contents" ]
api "$url/api/v1/search?q=survive" | grep -q '"title":"Important"'
after=$(docker compose exec -T postgres psql -U diffluence_owner -d diffluence -tAc 'SELECT count(*) FROM audit_events')
[ "$after" -ge "$before" ]

# The keys came back halfway through their change: everything opens, and the change can finish.
docker compose exec -T diffluence diffluence key check --deep >/dev/null
docker compose exec -T diffluence diffluence key reseal >/dev/null
docker compose exec -T diffluence diffluence key check --require-current >/dev/null
docker compose exec -T diffluence diffluence key retire >/dev/null
docker compose up -d --force-recreate --wait diffluence >/dev/null 2>&1
rm -f "$jar"
api -d '{"email":"admin@example.com","password":"correct horse battery staple"}' "$url/api/v1/auth/login" >/dev/null
[ "$(curl -sf -b "$jar" "$url/api/v1/attachments/$attachment/content?download=1")" = "attached file contents" ]
# The application role must still be unable to touch the audit log after a restore.
password=$(cat secrets/db_app_password.txt)
if docker compose exec -T -e PGPASSWORD="$password" postgres psql -h localhost -U diffluence_app -d diffluence -c "DELETE FROM audit_events" >/dev/null 2>&1; then
  echo "the application role can delete audit events after a restore"
  exit 1
fi

echo "5. the weekly restore check, on the backups of the service"
bash scripts/restore-check.sh "$(dirname "$service_backup")" "$service_key"
if bash scripts/restore-check.sh "$work/no-backups-here" "$service_key" >/dev/null 2>&1; then
  echo "the restore check passed without a backup"
  exit 1
fi
echo "restore drill passed in ${took}s: page, attachment, search, login, audit chain, role separation, a key change finished after the restore, a backup noted while the node was down, and the backup service"
