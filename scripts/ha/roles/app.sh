#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# An app node of the HA topology (spec 062): the diffluence package as role api or worker,
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
install -o diffluence -g diffluence -m 600 "$key" /var/lib/diffluence/master.key

umask 027
cat >/etc/diffluence/diffluence.env <<ENV
DIFFLUENCE_ROLE=$role
DIFFLUENCE_PUBLIC_URL=http://wiki.ha.test
DIFFLUENCE_HTTP_ADDR=0.0.0.0:8080
DIFFLUENCE_DATABASE_URL=postgres://diffluence_app:$HA_PG_APP_PASSWORD@$HA_EDGE:6432/diffluence?sslmode=disable&pool_max_conns=20
DIFFLUENCE_MIGRATE_DATABASE_URL=postgres://diffluence_owner:$HA_PG_OWNER_PASSWORD@$HA_EDGE:5000/diffluence?sslmode=disable
DIFFLUENCE_NATS_URL=nats://$HA_NATS1:4222,nats://$HA_NATS2:4222,nats://$HA_NATS3:4222
DIFFLUENCE_UPGRADE_BACKUP_MAX_AGE=off
DIFFLUENCE_LOG_LEVEL=info
ENV
chgrp diffluence /etc/diffluence/diffluence.env
# shellcheck disable=SC1091 # written just above, on the VM
(set -a && . /etc/diffluence/diffluence.env && set +a && diffluence config check --offline)
systemctl enable diffluence >/dev/null
systemctl restart diffluence
echo "$(hostname): diffluence started as $role"
