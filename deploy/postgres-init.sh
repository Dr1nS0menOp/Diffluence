#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
# Runs once, when the database is created. Makes the role the application uses.
# It owns nothing, so it cannot disable the triggers that protect the audit log.
set -eu
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" \
  -v app_password="$(cat /run/secrets/db_app_password)" <<'SQL'
CREATE ROLE diffluence_app LOGIN PASSWORD :'app_password';
GRANT CONNECT ON DATABASE diffluence TO diffluence_app;
GRANT USAGE ON SCHEMA public TO diffluence_app;
SQL
