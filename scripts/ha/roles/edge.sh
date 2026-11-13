#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# The edge node of the HA topology (spec 062): HAProxy in front of the PostgreSQL leader,
# the replicas and the api nodes, and PgBouncer in transaction mode in front of HAProxy.
# Run as root on the edge VM after the database nodes have a leader.
# Usage: bash roles/edge.sh
set -eu
# shellcheck source=common.sh
. "$(dirname "$0")/common.sh"
need_root
export HA_DB1 HA_DB2 HA_DB3 HA_API1 HA_API2

apt-get install -y -q haproxy pgbouncer postgresql-client-16 >/dev/null
render haproxy.cfg.tmpl /etc/haproxy/haproxy.cfg HA_DB1 HA_DB2 HA_DB3 HA_API1 HA_API2
haproxy -c -f /etc/haproxy/haproxy.cfg
systemctl restart haproxy

# The database and its two roles, made once on the leader through HAProxy. Passwords reach
# psql through the environment (\getenv), so they never appear in a process list.
export PGPASSWORD=$HA_PG_SUPERUSER_PASSWORD HA_PG_OWNER_PASSWORD HA_PG_APP_PASSWORD
psql -h 127.0.0.1 -p 5000 -U postgres -d postgres -v ON_ERROR_STOP=1 <<'SQL'
\getenv owner_pw HA_PG_OWNER_PASSWORD
\getenv app_pw HA_PG_APP_PASSWORD
SELECT 'CREATE ROLE diffluence_owner LOGIN' WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'diffluence_owner')\gexec
SELECT 'CREATE ROLE diffluence_app LOGIN' WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'diffluence_app')\gexec
ALTER ROLE diffluence_owner PASSWORD :'owner_pw';
ALTER ROLE diffluence_app PASSWORD :'app_pw';
SELECT 'CREATE DATABASE diffluence OWNER diffluence_owner' WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = 'diffluence')\gexec
SQL
unset PGPASSWORD

render pgbouncer.ini.tmpl /etc/pgbouncer/pgbouncer.ini
# The app's role only: migrations go to HAProxy's port 5000 directly, as the owner.
umask 077
printf '"%s" "%s"\n' diffluence_app "$HA_PG_APP_PASSWORD" >/etc/pgbouncer/userlist.txt
chown postgres:postgres /etc/pgbouncer/userlist.txt
systemctl restart pgbouncer
echo "edge: HAProxy on 5000 (leader), 5001 (replicas), 80 (app), 8404 (stats); PgBouncer on 6432"
