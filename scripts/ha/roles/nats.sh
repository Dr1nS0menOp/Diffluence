#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# A NATS JetStream cluster member of the HA topology (spec 062), from the pinned release
# with its checksum checked. Run as root on nats1, nats2 and nats3.
# Usage: bash roles/nats.sh diffluence-ha-nats1
set -eu
# shellcheck source=common.sh
. "$(dirname "$0")/common.sh"
need_root
NODE_NAME=${1:?usage: nats.sh <node name>}
NODE_ADDR=$(node_addr "$NODE_NAME")
[ -n "$NODE_ADDR" ] || { echo "no address for $NODE_NAME" >&2; exit 1; }

case "${HA_NATS_SYNC:-default}" in
  default) NATS_SYNC_LINE="" ;;
  1s) NATS_SYNC_LINE="sync_interval: 1s" ;;
  always) NATS_SYNC_LINE="sync_interval: always" ;;
  *) echo "HA_NATS_SYNC must be default, 1s or always" >&2; exit 1 ;;
esac
export NODE_NAME NATS_SYNC_LINE HA_NATS1 HA_NATS2 HA_NATS3

v=$HA_NATS_VERSION
base=https://github.com/nats-io/nats-server/releases/download/v$v
tmp=$(mktemp -d)
curl -fsSL -o "$tmp/nats.tar.gz" "$base/nats-server-v$v-linux-amd64.tar.gz"
curl -fsSL -o "$tmp/SHA256SUMS" "$base/SHA256SUMS"
expected=$(awk -v f="nats-server-v$v-linux-amd64.tar.gz" '$2 == f {print $1}' "$tmp/SHA256SUMS")
actual=$(sha256sum "$tmp/nats.tar.gz" | cut -d' ' -f1)
if [ -z "$expected" ] || [ "$expected" != "$actual" ]; then
  echo "checksum of the NATS release does not match" >&2
  exit 1
fi
tar -xzf "$tmp/nats.tar.gz" -C "$tmp"
install -m 755 "$tmp/nats-server-v$v-linux-amd64/nats-server" /usr/local/bin/nats-server
rm -rf "$tmp"

id nats >/dev/null 2>&1 || useradd --system --home /var/lib/nats --shell /usr/sbin/nologin nats
install -d -o nats -g nats -m 750 /var/lib/nats
install -d -m 755 /etc/nats
render nats.conf.tmpl /etc/nats/nats.conf NODE_NAME NATS_SYNC_LINE HA_NATS1 HA_NATS2 HA_NATS3
nats-server -t -c /etc/nats/nats.conf

cat >/etc/systemd/system/nats.service <<'UNIT'
[Unit]
Description=NATS JetStream cluster member (spec 062)
After=network-online.target
Wants=network-online.target

[Service]
User=nats
ExecStart=/usr/local/bin/nats-server -c /etc/nats/nats.conf
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
UNIT
systemctl daemon-reload
systemctl enable nats >/dev/null
systemctl restart nats
echo "$NODE_NAME: NATS started with sync policy ${HA_NATS_SYNC:-default}"
