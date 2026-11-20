#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# An app node of the HA topology (spec 062): the kenwerk package as role api or worker,
# reading through PgBouncer, migrating straight to the leader, with the three NATS nodes
# and the one master.key all nodes share (OPS-011). Run as root with the .deb and the key
# copied over, api1 first: it runs the migrations before the others start.
# Usage: bash roles/app.sh <api|worker> <path to .deb> <path to master.key>
set -eu
# shellcheck source=common.sh
. "$(dirname "$0")/common.sh"
need_root
role=${1:?usage: app.sh <api|worker> <deb> <master.key>}
deb=${2:?usage: app.sh <api|worker> <deb> <master.key>}
key=${3:?usage: app.sh <api|worker> <deb> <master.key>}
case "$role" in api | worker) ;; *) echo "role must be api or worker" >&2; exit 1 ;; esac
[ -f "$deb" ] || { echo "no package at $deb" >&2; exit 1; }
[ -s "$key" ] || { echo "no master.key at $key" >&2; exit 1; }

apt-get install -y -q "$deb" >/dev/null
install -o kenwerk -g kenwerk -m 600 "$key" /var/lib/kenwerk/master.key

umask 027
cat >/etc/kenwerk/kenwerk.env <<ENV
KENWERK_ROLE=$role
KENWERK_PUBLIC_URL=http://wiki.ha.test
KENWERK_HTTP_ADDR=0.0.0.0:8080
KENWERK_DATABASE_URL=postgres://kenwerk_app:$HA_PG_APP_PASSWORD@$HA_EDGE:6432/kenwerk?sslmode=disable&pool_max_conns=20
KENWERK_MIGRATE_DATABASE_URL=postgres://kenwerk_owner:$HA_PG_OWNER_PASSWORD@$HA_EDGE:5000/kenwerk?sslmode=disable
KENWERK_NATS_URL=nats://$HA_NATS1:4222,nats://$HA_NATS2:4222,nats://$HA_NATS3:4222
KENWERK_NATS_REPLICAS=3
KENWERK_UPGRADE_BACKUP_MAX_AGE=off
KENWERK_LOG_LEVEL=info
ENV
chgrp kenwerk /etc/kenwerk/kenwerk.env
# shellcheck disable=SC1091 # written just above, on the VM
(set -a && . /etc/kenwerk/kenwerk.env && set +a && kenwerk config check --offline)
systemctl enable kenwerk >/dev/null
systemctl restart kenwerk
echo "$(hostname): kenwerk started as $role"
