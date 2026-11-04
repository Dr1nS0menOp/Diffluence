#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
# Creates the secret files compose.yaml needs. It never overwrites existing ones.
set -eu
cd "$(dirname "$0")/.."
umask 077
mkdir -p secrets

new_password() { od -An -tx1 -N24 /dev/urandom | tr -d ' \n'; }

[ -f secrets/db_owner_password.txt ] || new_password >secrets/db_owner_password.txt
[ -f secrets/db_app_password.txt ] || new_password >secrets/db_app_password.txt
owner=$(cat secrets/db_owner_password.txt)
app=$(cat secrets/db_app_password.txt)
[ -f secrets/migrate_database_url.txt ] ||
  printf 'postgres://diffluence_owner:%s@postgres:5432/diffluence?sslmode=disable' "$owner" >secrets/migrate_database_url.txt
[ -f secrets/database_url.txt ] ||
  printf 'postgres://diffluence_app:%s@postgres:5432/diffluence?sslmode=disable' "$app" >secrets/database_url.txt
# The containers read these as a non-root user.
chmod 644 secrets/*.txt
echo "secrets are in ./secrets. Keep this folder out of git and back it up."
