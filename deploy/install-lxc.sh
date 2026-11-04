#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
# Installs Diffluence in a Debian or Ubuntu LXC container (or any systemd host)
# with a local PostgreSQL. Run as root, next to the diffluence binary:
#   sh install-lxc.sh https://wiki.example.com
set -eu
public_url=${1:?usage: install-lxc.sh <public url, like https://wiki.example.com>}
[ -f ./diffluence ] || { echo "put the diffluence binary next to this script"; exit 1; }

apt-get update -qq
apt-get install -y -qq postgresql

new_password() { od -An -tx1 -N24 /dev/urandom | tr -d ' \n'; }
owner=$(new_password)
app=$(new_password)

id diffluence >/dev/null 2>&1 || useradd --system --home-dir /var/lib/diffluence --shell /usr/sbin/nologin diffluence

su postgres -c psql <<SQL
CREATE ROLE diffluence_owner LOGIN PASSWORD '$owner';
CREATE ROLE diffluence_app LOGIN PASSWORD '$app';
CREATE DATABASE diffluence OWNER diffluence_owner;
SQL

install -m 0755 ./diffluence /usr/local/bin/diffluence
install -d -m 0750 -o root -g diffluence /etc/diffluence
umask 027
cat >/etc/diffluence/diffluence.env <<ENV
DIFFLUENCE_ROLE=all
DIFFLUENCE_HTTP_ADDR=127.0.0.1:8080
DIFFLUENCE_PUBLIC_URL=$public_url
DIFFLUENCE_DATA_DIR=/var/lib/diffluence
DIFFLUENCE_DATABASE_URL=postgres://diffluence_app:$app@127.0.0.1:5432/diffluence?sslmode=disable
DIFFLUENCE_MIGRATE_DATABASE_URL=postgres://diffluence_owner:$owner@127.0.0.1:5432/diffluence?sslmode=disable
ENV
chgrp diffluence /etc/diffluence/diffluence.env

install -m 0644 "$(dirname "$0")/systemd/diffluence.service" /etc/systemd/system/diffluence.service
systemctl daemon-reload
systemctl enable --now diffluence

echo "Diffluence runs on 127.0.0.1:8080. Put a TLS reverse proxy in front of it."
echo "Create the first administrator:"
echo "  set -a; . /etc/diffluence/diffluence.env; set +a"
echo "  printf '%s\\n' 'a long password' | diffluence user create-admin --email you@example.com --name You"
