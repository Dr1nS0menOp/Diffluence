#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# Backs up a Docker Compose installation into one folder, encrypted with AES-256.
# Run it from the folder with compose.yaml:
#   bash scripts/backup.sh /path/to/backups /somewhere/else/backup.key
# Plain data never reaches the disk: everything streams through gpg. See
# docs/operations/backup-restore.md. Every later stage of a pipe here reads all of
# its input, so pipefail is safe, and it stops a failed dump from looking like a
# good backup.
set -euo pipefail
umask 077
target=${1:?usage: backup.sh <folder> <key file>}
key=${2:?usage: backup.sh <folder> <key file>}

command -v gpg >/dev/null || { echo "gpg is needed to encrypt the backup" >&2; exit 1; }
[ -s "$key" ] || { echo "no key in $key: make one, see docs/operations/backup-restore.md" >&2; exit 1; }
mkdir -p "$target"
case "$(realpath "$key")/" in
  "$(realpath "$target")"/*) echo "the key may not sit with the backups it opens" >&2; exit 1 ;;
esac
ls secrets/*.txt >/dev/null

stamp=$(date -u +%Y%m%dT%H%M%SZ)
dir="$target/diffluence-$stamp"
mkdir -p "$dir"

encrypt_to() {
  gpg --batch --yes --quiet --no-symkey-cache --pinentry-mode loopback --passphrase-file "$key" \
    --symmetric --cipher-algo AES256 --s2k-digest-algo SHA512 --compress-algo none --output "$dir/$1"
}

# The database first: a file that appears after the dump is harmless, but a page
# pointing at a file that is not in the backup is not.
docker compose exec -T postgres pg_dump -U diffluence_owner -d diffluence -Fc | encrypt_to database.dump.gpg

# Stored files and the encryption key, read through a throw-away container.
volume=$(docker inspect --format '{{range .Mounts}}{{if eq .Destination "/var/lib/diffluence"}}{{.Name}}{{end}}{{end}}' "$(docker compose ps -q diffluence)")
docker run --rm -v "$volume:/data:ro" busybox tar -C /data -czf - --exclude=./master.key --exclude=./nats . | encrypt_to files.tar.gz.gpg
docker run --rm -v "$volume:/data:ro" busybox cat /data/master.key | encrypt_to master.key.gpg
tar -C secrets -cf - . | encrypt_to secrets.tar.gpg

# Relative names, so the backup can be checked after moving it to another machine.
(cd "$dir" && sha256sum -- * >SHA256SUMS)
echo "backup written to $dir"
echo "keep $key away from the backups: without it they cannot be restored, and with them it opens everything."
