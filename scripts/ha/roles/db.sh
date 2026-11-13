#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# A database node of the HA topology (spec 062): PostgreSQL 16, an etcd member and Patroni
# with the watchdog required. Run as root on db1, db2 and db3, with this folder copied over.
# Usage: bash roles/db.sh diffluence-ha-db1
set -eu
# shellcheck source=common.sh
. "$(dirname "$0")/common.sh"
need_root
NODE_NAME=${1:?usage: db.sh <node name>}
NODE_ADDR=$(node_addr "$NODE_NAME")
[ -n "$NODE_ADDR" ] || { echo "no address for $NODE_NAME" >&2; exit 1; }
export NODE_NAME NODE_ADDR HA_DB1 HA_DB2 HA_DB3 HA_PATRONI_REST_PASSWORD \
  HA_PG_SUPERUSER_PASSWORD HA_PG_REPLICATION_PASSWORD

apt-get install -y -q postgresql-16 patroni etcd-server python3-etcd3 >/dev/null
# Patroni starts PostgreSQL itself; the packaged cluster must not run beside it.
systemctl disable --now postgresql >/dev/null 2>&1 || true
pg_dropcluster --stop 16 main 2>/dev/null || true

# etcd: one member per database node, three in all (D13 of spec 048).
cat >/etc/default/etcd <<ETCD
ETCD_NAME=$NODE_NAME
ETCD_DATA_DIR=/var/lib/etcd/diffluence
ETCD_LISTEN_PEER_URLS=http://$NODE_ADDR:2380
ETCD_LISTEN_CLIENT_URLS=http://$NODE_ADDR:2379,http://127.0.0.1:2379
ETCD_INITIAL_ADVERTISE_PEER_URLS=http://$NODE_ADDR:2380
ETCD_ADVERTISE_CLIENT_URLS=http://$NODE_ADDR:2379
ETCD_INITIAL_CLUSTER=diffluence-ha-db1=http://$HA_DB1:2380,diffluence-ha-db2=http://$HA_DB2:2380,diffluence-ha-db3=http://$HA_DB3:2380
ETCD_INITIAL_CLUSTER_STATE=new
ETCD_INITIAL_CLUSTER_TOKEN=diffluence-ha
ETCD
systemctl enable etcd >/dev/null
systemctl restart etcd

# The watchdog: Patroni must own it, so a hung Patroni resets the machine.
modprobe softdog
chown postgres /dev/watchdog
echo 'KERNEL=="watchdog", OWNER="postgres"' >/etc/udev/rules.d/61-watchdog.rules

install -d -o postgres -g postgres -m 700 /var/lib/postgresql/16/ha
install -d -m 755 /etc/patroni
render patroni.yml.tmpl /etc/patroni/config.yml NODE_NAME NODE_ADDR HA_DB1 HA_DB2 HA_DB3 \
  HA_PATRONI_REST_PASSWORD HA_PG_SUPERUSER_PASSWORD HA_PG_REPLICATION_PASSWORD
chown postgres:postgres /etc/patroni/config.yml
chmod 600 /etc/patroni/config.yml
patroni --validate-config /etc/patroni/config.yml
systemctl enable patroni >/dev/null
systemctl restart patroni
echo "$NODE_NAME: etcd and Patroni started; check with: patronictl -c /etc/patroni/config.yml list"
