#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# The backup service of compose.yaml (spec 054): the steps of scripts/backup.sh, run from
# inside Compose on an interval, so an installation has backups without anyone remembering.
#   diffluence-backup loop   back up every DIFFLUENCE_BACKUP_INTERVAL (the default)
#   diffluence-backup once   make one backup and exit with its result
# It reads the database as the owner, the data volume and ./secrets read-only, and the key
# from /key/backup.key. Backups go to /backups, encrypted with gpg as backup.sh does, so
# scripts/restore.sh restores them.
#
# Every step checks its own result: these functions also run on the left of ||, where
# set -e does not apply.
set -uo pipefail
umask 077

target=/backups
key=/key/backup.key
interval=${DIFFLUENCE_BACKUP_INTERVAL:-4h}
keep=${DIFFLUENCE_BACKUP_KEEP:-42}
export GNUPGHOME=/tmp/gnupg

log() { printf '%s backup: %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"; }

# seconds_of turns 90, 30s, 15m, 4h or 1d into seconds; nothing for anything else.
seconds_of() {
  case "$1" in
    *[!0-9smhd]* | '' | [smhd]*) return 1 ;;
    *d) echo $((${1%d} * 86400)) ;;
    *h) echo $((${1%h} * 3600)) ;;
    *m) echo $((${1%m} * 60)) ;;
    *s) echo "${1%s}" ;;
    *) echo "$1" ;;
  esac
}

# setup_problem names what stops a backup before it starts; nothing when all is in place.
setup_problem() {
  if [ ! -s "$key" ]; then
    echo "no backup key in the key folder: run sh scripts/setup-secrets.sh (docs/operations/backup-restore.md)"
  elif [ "$(stat -c %d:%i /key)" = "$(stat -c %d:%i "$target")" ]; then
    echo "the key folder is the backup folder: the key may not sit with the backups it opens"
  else
    case "$keep" in
      '' | *[!0-9]* | 0) echo "DIFFLUENCE_BACKUP_KEEP must be a whole number above 0, not $keep" ;;
    esac
  fi
}

# schema_ready is whether Diffluence has made its tables, so a backup can be noted.
schema_ready() {
  [ "$(psql --dbname="$(cat /run/secrets/migrate_database_url)" -tAc "SELECT to_regclass('audit_events') IS NOT NULL" 2>/dev/null)" = t ]
}

encrypt_to() {
  gpg --batch --yes --quiet --no-symkey-cache --pinentry-mode loopback --passphrase-file "$key" \
    --symmetric --cipher-algo AES256 --s2k-digest-algo SHA512 --compress-algo none --output "$1"
}

backup_once() {
  local problem stamp work dir extra
  problem=$(setup_problem)
  if [ -n "$problem" ]; then
    log "no backup made: $problem"
    return 1
  fi
  mkdir -p "$GNUPGHOME" || return 1
  stamp=$(date -u +%Y%m%dT%H%M%SZ)
  # A backup is written under a name no restore picks, and renamed once every part is in.
  rm -rf -- "$target"/.partial-diffluence-* || return 1
  work=$target/.partial-diffluence-$stamp
  dir=$target/diffluence-$stamp
  mkdir "$work" || return 1
  # The database first: a file that appears after the dump is harmless, but a page that
  # points at a file the backup lacks is not.
  pg_dump --dbname="$(cat /run/secrets/migrate_database_url)" -Fc | encrypt_to "$work/database.dump.gpg" || return 1
  tar -C /data -czf - --exclude=./master.key --exclude=./master.key.next --exclude=./master.key.previous \
    --exclude=./nats . | encrypt_to "$work/files.tar.gz.gpg" || return 1
  encrypt_to "$work/master.key.gpg" </data/master.key || return 1
  for extra in next previous; do
    if [ -f "/data/master.key.$extra" ]; then
      encrypt_to "$work/master.key.$extra.gpg" <"/data/master.key.$extra" || return 1
    fi
  done
  tar -C /secrets -cf - . | encrypt_to "$work/secrets.tar.gpg" || return 1
  (cd "$work" && sha256sum -- * >SHA256SUMS) || return 1
  # The operator who owns the backup folder restores from it with scripts/restore.sh.
  chown -R "$(stat -c %u:%g "$target")" "$work" || return 1
  mv "$work" "$dir" || return 1
  if ! diffluence backup note >/dev/null; then
    log "the backup in $dir is complete, but noting it in Diffluence failed: an upgrade that asks for a recent backup will refuse"
    return 1
  fi
  log "backup written to $dir and noted"
  prune
}

# prune keeps the newest DIFFLUENCE_BACKUP_KEEP backups.
prune() {
  local old
  find "$target" -mindepth 1 -maxdepth 1 -type d -name 'diffluence-*' | sort | head -n -"$keep" |
    while IFS= read -r old; do
      rm -rf -- "$old" && log "removed the old backup $old"
    done
}

# due_in is how many seconds remain before the next backup is due.
due_in() {
  local newest age
  newest=$(find "$target" -mindepth 1 -maxdepth 1 -type d -name 'diffluence-*' | sort | tail -n 1)
  [ -n "$newest" ] || { echo 0; return; }
  age=$(($(date +%s) - $(stat -c %Y "$newest")))
  echo $((age >= every ? 0 : every - age))
}

case "${1:-loop}" in
  once)
    backup_once
    ;;
  loop)
    if ! every=$(seconds_of "$interval"); then
      log "DIFFLUENCE_BACKUP_INTERVAL must look like 4h, 30m or 1d, not $interval"
      exit 2
    fi
    log "backing up every $interval, keeping the newest $keep"
    while true; do
      if ! schema_ready; then
        log "waiting for Diffluence to make its tables before the first backup"
        sleep 60
        continue
      fi
      wait=$(due_in)
      if [ "$wait" -gt 0 ]; then
        sleep "$wait"
        continue
      fi
      # This loop is where a failed backup is recorded and the next one tried: the health
      # page of Diffluence warns when no backup was noted for 26 hours.
      backup_once || log "the backup failed; the next try is in $interval"
      sleep "$every"
    done
    ;;
  *)
    echo "usage: diffluence-backup [loop|once]" >&2
    exit 2
    ;;
esac
