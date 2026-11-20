#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
# Installs Kenwerk in a Debian or Ubuntu LXC container (or any systemd host)
# with a local PostgreSQL. Run as root, next to the kenwerk binary:
#   sh install-lxc.sh https://wiki.example.com
set -eu
public_url=${1:?usage: install-lxc.sh <public url, like https://wiki.example.com>}
[ -f ./kenwerk ] || { echo "put the kenwerk binary next to this script"; exit 1; }

apt-get update -qq
apt-get install -y -qq postgresql

new_password() { od -An -tx1 -N24 /dev/urandom | tr -d ' \n'; }
owner=$(new_password)
app=$(new_password)

id kenwerk >/dev/null 2>&1 || useradd --system --home-dir /var/lib/kenwerk --shell /usr/sbin/nologin kenwerk

su postgres -c psql <<SQL
CREATE ROLE kenwerk_owner LOGIN PASSWORD '$owner';
CREATE ROLE kenwerk_app LOGIN PASSWORD '$app';
CREATE DATABASE kenwerk OWNER kenwerk_owner;
SQL

install -m 0755 ./kenwerk /usr/local/bin/kenwerk
install -d -m 0750 -o root -g kenwerk /etc/kenwerk
umask 027
cat >/etc/kenwerk/kenwerk.env <<ENV
KENWERK_ROLE=all
KENWERK_HTTP_ADDR=127.0.0.1:8080
KENWERK_PUBLIC_URL=$public_url
KENWERK_DATA_DIR=/var/lib/kenwerk
KENWERK_DATABASE_URL=postgres://kenwerk_app:$app@127.0.0.1:5432/kenwerk?sslmode=disable
KENWERK_MIGRATE_DATABASE_URL=postgres://kenwerk_owner:$owner@127.0.0.1:5432/kenwerk?sslmode=disable
ENV
chgrp kenwerk /etc/kenwerk/kenwerk.env

install -m 0644 "$(dirname "$0")/systemd/kenwerk.service" /etc/systemd/system/kenwerk.service
systemctl daemon-reload
systemctl enable --now kenwerk

echo "Kenwerk runs on 127.0.0.1:8080. Put a TLS reverse proxy in front of it."
echo "Create the first administrator:"
echo "  set -a; . /etc/kenwerk/kenwerk.env; set +a"
echo "  printf '%s\\n' 'a long password' | kenwerk user create-admin --email you@example.com --name You"
