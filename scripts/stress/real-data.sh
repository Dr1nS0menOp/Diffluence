#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# Restores a dump of the seeded stress-test data into a throw-away PostgreSQL 16 and runs
# the real-data checks against it, from the development toolbox (spec 061).
#
#   bash scripts/stress/real-data.sh /path/to/seeded.dump
#
# REALDATA_PACKAGE and REALDATA_RUN choose other checks than those of the feed, such as
# REALDATA_PACKAGE=./internal/app/ REALDATA_RUN=SearchOnRealData (spec 080), and
# REALDATA_NAME another name for the database, so builders on one machine keep theirs apart.
#
# Make the dump on the app machine of the stress test with
#   docker compose exec -T postgres pg_dump -U kenwerk_owner -d kenwerk -Fc > seeded.dump
# The dump holds only generated people and pages; still, keep it out of the repository.
# Generated data in the test database is too small and too even to show what the planner
# does with a real data set, so every change to a hot read is measured here as well.
set -u
dump=${1:?usage: real-data.sh <seeded.dump>}
[ -r "$dump" ] || { echo "cannot read $dump" >&2; exit 1; }
name=${REALDATA_NAME:-kenwerk-realdata}
network=$(docker inspect --format '{{range $k, $v := .NetworkSettings.Networks}}{{$k}}{{end}}' \
  "$(docker compose -f compose.dev.yaml ps -q dev)") || { echo "start the toolbox first: scripts/dev up" >&2; exit 1; }

if ! docker inspect "$name" >/dev/null 2>&1; then
  docker run -d --name "$name" --network "$network" -e POSTGRES_PASSWORD=realdata \
    -v "$(realpath "$(dirname "$dump")"):/dump:ro" postgres:16 \
    -c shared_buffers=1GB -c work_mem=32MB -c effective_cache_size=4GB >/dev/null || exit 1
  for _ in $(seq 1 60); do docker exec "$name" pg_isready -U postgres >/dev/null 2>&1 && break; sleep 1; done
  docker exec "$name" psql -U postgres -qc "CREATE ROLE kenwerk_owner LOGIN SUPERUSER" \
    -c "CREATE ROLE kenwerk_app LOGIN" -c "CREATE DATABASE kenwerk OWNER kenwerk_owner" || exit 1
  docker exec "$name" pg_restore -U postgres -d kenwerk -j 4 --no-owner --role=kenwerk_owner \
    "/dump/$(basename "$dump")" || echo "pg_restore reported warnings; the checks still run"
  docker exec "$name" psql -U postgres -d kenwerk -qc "ANALYZE" || exit 1
else
  docker network connect "$network" "$name" 2>/dev/null || true
fi

bash scripts/dev env KENWERK_REALDATA_URL="postgres://postgres:realdata@$name:5432/kenwerk?sslmode=disable" \
  go test -count=1 -run "${REALDATA_RUN:-RealData}" -v "${REALDATA_PACKAGE:-./internal/feed/}"
status=$?
echo "the database stays as $name for more runs; remove it with: docker rm -f $name"
exit $status
