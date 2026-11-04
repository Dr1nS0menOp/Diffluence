#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
# Restores a backup onto a clean Docker Compose installation.
# Run it from the folder with compose.yaml:  sh scripts/restore.sh /path/to/backup
set -eu
src=${1:?usage: restore.sh <backup folder>}
(cd "$src" && sha256sum --quiet -c SHA256SUMS)

mkdir -p secrets
cp "$src"/db_owner_password.txt "$src"/db_app_password.txt "$src"/database_url.txt "$src"/migrate_database_url.txt secrets/
chmod 644 secrets/*.txt

docker compose up -d --wait postgres
docker compose exec -T postgres pg_restore -U diffluence_owner -d diffluence --clean --if-exists --exit-on-error <"$src/database.dump"

# Create the volume, without starting the server yet.
docker compose create diffluence
volume=$(docker inspect --format '{{range .Mounts}}{{if eq .Destination "/var/lib/diffluence"}}{{.Name}}{{end}}{{end}}' "$(docker compose ps -aq diffluence)")
abs=$(cd "$src" && pwd)
docker run --rm -v "$volume:/data" -v "$abs:/backup:ro" busybox \
  sh -c 'tar -C /data -xzf /backup/files.tar.gz && cp /backup/master.key /data/master.key && chmod 600 /data/master.key && chown -R 65532:65532 /data'

docker compose up -d --wait diffluence
docker compose exec -T diffluence diffluence audit verify
echo "restore finished"
