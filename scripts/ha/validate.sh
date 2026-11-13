#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# Checks the HA scripts and configs of spec 062 without a test host: bash syntax, the
# Python ledger, and the validators of HAProxy, NATS and Patroni in throw-away containers
# with names of their own, removed afterwards. Needs Docker.
set -u
here=$(cd "$(dirname "$0")" && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
fail=0
step() { echo "== $1"; }

step "bash -n"
for f in "$here"/*.sh "$here"/roles/*.sh "$here"/tests/*.sh; do
  bash -n "$f" || { echo "syntax: $f"; fail=1; }
done

step "python"
# Parsed only, so no compiled file is left beside it.
python3 -c 'import ast, sys; ast.parse(open(sys.argv[1]).read())' "$here/tests/ledger.py" || fail=1

# Sample values, only to fill in the templates for the validators.
cat >"$work/ha.local.env" <<'ENV'
HA_DB1=10.0.0.11
HA_DB2=10.0.0.12
HA_DB3=10.0.0.13
HA_NATS1=10.0.0.21
HA_NATS2=10.0.0.22
HA_NATS3=10.0.0.23
HA_API1=10.0.0.41
HA_API2=10.0.0.42
HA_PATRONI_REST_PASSWORD=sample
HA_PG_SUPERUSER_PASSWORD=sample
HA_PG_REPLICATION_PASSWORD=sample
ENV
set -a
# shellcheck disable=SC1091
. "$work/ha.local.env"
set +a
NODE_NAME=diffluence-ha-db1 NODE_ADDR=10.0.0.11 NATS_SYNC_LINE="sync_interval: always" \
  envsubst <"$here/conf/patroni.yml.tmpl" >"$work/patroni.yml"
NODE_NAME=diffluence-ha-nats1 NATS_SYNC_LINE="sync_interval: always" \
  envsubst <"$here/conf/nats.conf.tmpl" >"$work/nats.conf"
envsubst <"$here/conf/haproxy.cfg.tmpl" >"$work/haproxy.cfg"
chmod 755 "$work"
chmod 644 "$work"/*

step "haproxy -c"
docker run --rm --name "ha-validate-haproxy-$$" -v "$work:/w:ro" haproxy:2.8 haproxy -c -f /w/haproxy.cfg || fail=1

step "nats-server -t"
docker run --rm --name "ha-validate-nats-$$" -v "$work:/w:ro" nats:2.10 -t -c /w/nats.conf || fail=1

step "patroni --validate-config"
# Patroni checks that etcd answers, so a throw-away etcd stands in for the three members.
net="ha-validate-$$"
docker network create "$net" >/dev/null
docker run -d --rm --name "ha-validate-etcd-$$" --network "$net" --network-alias etcd \
  quay.io/coreos/etcd:v3.5.17 etcd --listen-client-urls http://0.0.0.0:2379 \
  --advertise-client-urls http://etcd:2379 >/dev/null
sed 's|^  hosts: .*|  hosts: etcd:2379|' "$work/patroni.yml" >"$work/patroni-local.yml"
chmod 644 "$work/patroni-local.yml"
docker run --rm --name "ha-validate-patroni-$$" --network "$net" -v "$work:/w:ro" ubuntu:24.04 sh -c \
  'apt-get update -qq >/dev/null && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq patroni python3-etcd3 postgresql-16 >/dev/null 2>&1 && patroni --validate-config /w/patroni-local.yml' || fail=1
docker rm -f "ha-validate-etcd-$$" >/dev/null 2>&1
docker network rm "$net" >/dev/null 2>&1

[ "$fail" = 0 ] && echo "all HA scripts and configs are valid"
exit "$fail"
