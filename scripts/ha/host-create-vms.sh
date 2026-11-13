#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# Creates the VMs of the HA topology (spec 062) on a Proxmox VE host with qm. The cloud
# image is imported once into template 9000; every VM is a linked clone of it, which takes
# seconds instead of the half hour an import took in the first stress test. Each VM gets a
# static address from ha.local.env and the common cloud-init. Run as root on the host.
# It refuses nothing that exists: existing ids are skipped.
# Usage: bash host-create-vms.sh <path to the cloud image> <ssh public key file>
set -eu
here=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=topology.env
. "$here/topology.env"
# shellcheck disable=SC1091 # written by the operator, never committed
. "$here/ha.local.env"
image=${1:?usage: host-create-vms.sh <cloud image> <ssh public key>}
pubkey=${2:?usage: host-create-vms.sh <cloud image> <ssh public key>}
[ -f "$image" ] || { echo "no image at $image" >&2; exit 1; }
[ -f "$pubkey" ] || { echo "no public key at $pubkey" >&2; exit 1; }
command -v qm >/dev/null || { echo "qm not found: run this on the test host" >&2; exit 1; }
template=9000

mkdir -p /var/lib/vz/snippets
cp "$here/cloud-init/common.yaml" /var/lib/vz/snippets/diffluence-ha-common.yaml

if ! qm status "$template" >/dev/null 2>&1; then
  echo "== template $template"
  qm create "$template" --name diffluence-ha-template --cores 1 --memory 1024 --cpu host \
    --net0 virtio,bridge=vmbr0 --scsihw virtio-scsi-single --agent enabled=1 \
    --serial0 socket --vga serial0 --ostype l26
  qm importdisk "$template" "$image" "$HA_STORAGE" --format raw >/dev/null
  qm set "$template" --scsi0 "$HA_STORAGE:vm-$template-disk-0,iothread=1,discard=on" --boot order=scsi0 >/dev/null
  qm set "$template" --ide2 "$HA_STORAGE:cloudinit" >/dev/null
  qm template "$template"
fi

address_of() {
  local key
  key="HA_$(echo "${1#diffluence-ha-}" | tr '[:lower:]' '[:upper:]')"
  printf '%s' "${!key:-}"
}

echo "$HA_VMS" | while read -r id name role vcpu mem disk; do
  [ -n "${id:-}" ] || continue
  [ "$role" = load ] && continue # the load VM of the stress tests is reused as it is
  if qm status "$id" >/dev/null 2>&1; then
    echo "VM $id exists, skipped"
    continue
  fi
  addr=$(address_of "$name")
  [ -n "$addr" ] || { echo "no address for $name in ha.local.env" >&2; exit 1; }
  echo "== $id $name ($role, $vcpu vCPU, $mem MB, $disk GB)"
  qm clone "$template" "$id" --name "$name" >/dev/null
  qm set "$id" --cores "$vcpu" --memory "$mem" --sshkeys "$pubkey" \
    --ipconfig0 "ip=$addr/$HA_PREFIX,gw=$HA_GATEWAY" \
    --cicustom "vendor=local:snippets/diffluence-ha-common.yaml" >/dev/null
  qm resize "$id" scsi0 "${disk}G" >/dev/null
  qm start "$id"
done
echo "created; wait for cloud-init, then run the role scripts (spec 062, session S1)"
