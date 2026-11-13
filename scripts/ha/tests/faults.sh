#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# The faults of spec 062, one per call, so each test of the plan names exactly what it
# does. Runs on the test host as root (it needs qm for the hard ones) and reaches the VMs
# over ssh as the operator's user with sudo.
# Usage: bash faults.sh <fault> <target> [seconds]
#   kill <vm-name> <service>   SIGKILL the service (patroni, postgresql, nats, diffluence, haproxy, pgbouncer)
#   stop-primary               SIGSTOP the PostgreSQL leader's postmaster for [seconds], then SIGCONT
#   pause-vm <vm-id>           freeze the whole VM for [seconds] (qm suspend), then resume
#   reset-vm <vm-id>           hard reset, as a power cut (qm reset)
#   cut <vm-name> <peer-addr>  drop all traffic between the VM and one address for [seconds]
set -eu
here=$(cd "$(dirname "$0")/.." && pwd)
# shellcheck source=../topology.env
. "$here/topology.env"
# shellcheck disable=SC1091 # written by the operator, never committed
. "$here/ha.local.env"
fault=${1:?usage: faults.sh <fault> ...}
user=${HA_SSH_USER:-ubuntu}

addr_of() {
  local key
  key="HA_$(echo "${1#diffluence-ha-}" | tr '[:lower:]' '[:upper:]')"
  printf '%s' "${!key:-}"
}
on() { ssh -o BatchMode=yes -o ConnectTimeout=5 "$user@$(addr_of "$1")" "sudo sh -c '$2'"; }
stamp() { date -u +%FT%T.%3NZ; }

case "$fault" in
  kill)
    vm=${2:?vm}; svc=${3:?service}
    echo "$(stamp) kill $svc on $vm"
    on "$vm" "systemctl kill -s KILL $svc"
    ;;
  stop-primary)
    secs=${2:-60}
    leader=$(curl -fsS "http://$HA_DB1:8008/cluster" | jq -r '.members[] | select(.role == "leader") | .name')
    [ -n "$leader" ] || { echo "no leader found" >&2; exit 1; }
    echo "$(stamp) SIGSTOP the postmaster of $leader for ${secs}s"
    on "$leader" "kill -STOP \$(head -1 /var/lib/postgresql/16/ha/postmaster.pid)"
    sleep "$secs"
    on "$leader" "kill -CONT \$(head -1 /var/lib/postgresql/16/ha/postmaster.pid)"
    echo "$(stamp) SIGCONT $leader"
    ;;
  pause-vm)
    id=${2:?vm id}; secs=${3:-60}
    echo "$(stamp) suspend VM $id for ${secs}s"
    qm suspend "$id"
    sleep "$secs"
    qm resume "$id"
    echo "$(stamp) resumed VM $id"
    ;;
  reset-vm)
    id=${2:?vm id}
    echo "$(stamp) hard reset of VM $id"
    qm reset "$id"
    ;;
  cut)
    vm=${2:?vm}; peer=${3:?peer address}; secs=${4:-60}
    echo "$(stamp) cut $vm from $peer for ${secs}s"
    on "$vm" "nft add table inet ha; nft add chain inet ha cut { type filter hook input priority 0 \\; }; nft add rule inet ha cut ip saddr $peer drop; nft add chain inet ha cutout { type filter hook output priority 0 \\; }; nft add rule inet ha cutout ip daddr $peer drop"
    sleep "$secs"
    on "$vm" "nft delete table inet ha"
    echo "$(stamp) restored $vm to $peer"
    ;;
  *)
    echo "unknown fault $fault" >&2
    exit 1
    ;;
esac
