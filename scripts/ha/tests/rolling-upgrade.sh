#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# A rolling upgrade of the app nodes under load (spec 062, test T7): one node at a time is
# stopped (it drains: not ready first, then closes), gets the new package and starts, and
# the next waits until it is ready. Workers first, then the api nodes, as upgrade.md says.
# Runs on the operator's machine with ssh to the VMs.
# Usage: bash rolling-upgrade.sh <path to the new .deb>
set -eu
here=$(cd "$(dirname "$0")/.." && pwd)
# shellcheck disable=SC1091 # written by the operator, never committed
. "$here/ha.local.env"
deb=${1:?usage: rolling-upgrade.sh <new .deb>}
[ -f "$deb" ] || { echo "no package at $deb" >&2; exit 1; }
user=${HA_SSH_USER:-ubuntu}
stamp() { date -u +%FT%T.%3NZ; }

upgrade() {
  local addr=$1 name=$2
  echo "$(stamp) $name: copy and stop"
  scp -q -o BatchMode=yes "$deb" "$user@$addr:/tmp/diffluence-new.deb"
  ssh -o BatchMode=yes "$user@$addr" "sudo systemctl stop diffluence && sudo apt-get install -y -q /tmp/diffluence-new.deb >/dev/null && sudo systemctl start diffluence"
  for _ in $(seq 1 60); do
    if ssh -o BatchMode=yes "$user@$addr" "curl -fsS -o /dev/null http://127.0.0.1:8080/readyz"; then
      echo "$(stamp) $name: ready on $(ssh -o BatchMode=yes "$user@$addr" 'diffluence version')"
      return 0
    fi
    sleep 2
  done
  echo "$(stamp) $name did not become ready" >&2
  return 1
}

upgrade "$HA_WORK1" work1
upgrade "$HA_WORK2" work2
upgrade "$HA_API1" api1
upgrade "$HA_API2" api2
echo "$(stamp) all app nodes upgraded"
