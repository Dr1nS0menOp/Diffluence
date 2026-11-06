#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# Restores a backup onto a clean Docker Compose installation. Run it from the folder
# with compose.yaml:  bash scripts/restore.sh /path/to/backup /path/to/backup.key
# Every later stage of a pipe here reads all of its input, so pipefail is safe.
set -euo pipefail
src=${1:?usage: restore.sh <backup folder> <key file>}
key=${2:?usage: restore.sh <backup folder> <key file>}
command -v gpg >/dev/null || { echo "gpg is needed to open the backup" >&2; exit 1; }
(cd "$src" && sha256sum --quiet -c SHA256SUMS)

decrypt() {
  gpg --batch --quiet --no-symkey-cache --pinentry-mode loopback --passphrase-file "$key" --decrypt "$src/$1"
}

# A wrong key stops here, before anything has changed.
decrypt master.key.gpg >/dev/null

mkdir -p secrets
decrypt secrets.tar.gpg | tar -C secrets -xf -
chmod 644 secrets/*.txt

docker compose up -d --wait postgres
decrypt database.dump.gpg | docker compose exec -T postgres pg_restore -U diffluence_owner -d diffluence --clean --if-exists --exit-on-error

# Create the volume, without starting the server yet.
docker compose create diffluence
volume=$(docker inspect --format '{{range .Mounts}}{{if eq .Destination "/var/lib/diffluence"}}{{.Name}}{{end}}{{end}}' "$(docker compose ps -aq diffluence)")
decrypt files.tar.gz.gpg | docker run --rm -i -v "$volume:/data" busybox tar -C /data -xzf -
decrypt master.key.gpg | docker run --rm -i -v "$volume:/data" busybox \
  sh -c 'cat >/data/master.key && chmod 600 /data/master.key && chown -R 65532:65532 /data'

docker compose up -d --wait diffluence
docker compose exec -T diffluence diffluence audit verify
echo "restore finished"
