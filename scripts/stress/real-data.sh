#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# Restores a dump of the seeded stress-test data into a throw-away PostgreSQL 16 and runs
# the real-data checks against it, from the development toolbox (spec 061).
#
#   bash scripts/stress/real-data.sh /path/to/seeded.dump
#
# Make the dump on the app machine of the stress test with
#   docker compose exec -T postgres pg_dump -U diffluence_owner -d diffluence -Fc > seeded.dump
# The dump holds only generated people and pages; still, keep it out of the repository.
# Generated data in the test database is too small and too even to show what the planner
# does with a real data set, so every change to a hot read is measured here as well.
set -u
dump=${1:?usage: real-data.sh <seeded.dump>}
[ -r "$dump" ] || { echo "cannot read $dump" >&2; exit 1; }
name=diffluence-realdata
network=$(docker inspect --format '{{range $k, $v := .NetworkSettings.Networks}}{{$k}}{{end}}' \
  "$(docker compose -f compose.dev.yaml ps -q dev)") || { echo "start the toolbox first: scripts/dev up" >&2; exit 1; }

if ! docker inspect "$name" >/dev/null 2>&1; then
  docker run -d --name "$name" --network "$network" -e POSTGRES_PASSWORD=realdata \
    -v "$(realpath "$(dirname "$dump")"):/dump:ro" postgres:16 \
    -c shared_buffers=1GB -c work_mem=32MB -c effective_cache_size=4GB >/dev/null || exit 1
  for _ in $(seq 1 60); do docker exec "$name" pg_isready -U postgres >/dev/null 2>&1 && break; sleep 1; done
  docker exec "$name" psql -U postgres -qc "CREATE ROLE diffluence_owner LOGIN SUPERUSER" \
    -c "CREATE ROLE diffluence_app LOGIN" -c "CREATE DATABASE diffluence OWNER diffluence_owner" || exit 1
  docker exec "$name" pg_restore -U postgres -d diffluence -j 4 --no-owner --role=diffluence_owner \
    "/dump/$(basename "$dump")" || echo "pg_restore reported warnings; the checks still run"
  docker exec "$name" psql -U postgres -d diffluence -qc "ANALYZE" || exit 1
else
  docker network connect "$network" "$name" 2>/dev/null || true
fi

bash scripts/dev env DIFFLUENCE_REALDATA_URL="postgres://postgres:realdata@$name:5432/diffluence?sslmode=disable" \
  go test -count=1 -run RealData -v ./internal/feed/
status=$?
echo "the database stays as $name for more runs; remove it with: docker rm -f $name"
exit $status
