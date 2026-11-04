#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
# Backs up a Docker Compose installation into one folder. Run it from the folder
# with compose.yaml:  sh scripts/backup.sh /path/to/backups
# Copy the result off this machine. See docs/operations/backup-restore.md.
set -eu
target=${1:?usage: backup.sh <folder>}
stamp=$(date -u +%Y%m%dT%H%M%SZ)
dir="$target/diffluence-$stamp"
mkdir -p "$dir"
umask 077

# The database first: a file that appears after the dump is harmless, but a page
# pointing at a file that is not in the backup is not.
docker compose exec -T postgres pg_dump -U diffluence_owner -d diffluence -Fc >"$dir/database.dump"

# Stored files and the encryption key, read through a throw-away container.
volume=$(docker inspect --format '{{range .Mounts}}{{if eq .Destination "/var/lib/diffluence"}}{{.Name}}{{end}}{{end}}' "$(docker compose ps -q diffluence)")
abs=$(cd "$dir" && pwd)
docker run --rm -e OWNER="$(id -u):$(id -g)" -v "$volume:/data:ro" -v "$abs:/backup" busybox \
  sh -c 'tar -C /data -czf /backup/files.tar.gz --exclude=./master.key --exclude=./nats . &&
         cp /data/master.key /backup/master.key &&
         chown "$OWNER" /backup/files.tar.gz /backup/master.key'

cp -p secrets/*.txt "$dir/" 2>/dev/null || true
# Relative names, so the backup can be checked after moving it to another machine.
(cd "$dir" && sha256sum -- * >SHA256SUMS)
echo "backup written to $dir"
echo "keep master.key and the secrets separate from the rest: anyone with both can read TOTP seeds."
