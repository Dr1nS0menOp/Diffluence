#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# Shared by the role scripts of spec 062: loads the topology and the local settings, and
# fills in a template with only the variables named, so a stray $ in a config survives.
here=$(cd "$(dirname "$0")/.." && pwd)
# shellcheck source=../topology.env
. "$here/topology.env"
# shellcheck disable=SC1091 # written by the operator, never committed
. "$here/ha.local.env"

# node_addr <node name>: the node's address on the test network.
node_addr() {
  local key
  key="HA_$(echo "${1#diffluence-ha-}" | tr '[:lower:]' '[:upper:]')"
  printf '%s' "${!key:-}"
}

# render <template> <target> <VAR...>: writes the template with only those variables set.
render() {
  local template=$1 target=$2
  shift 2
  local vars=""
  local v
  for v in "$@"; do vars="$vars \${$v}"; done
  # shellcheck disable=SC2016 # envsubst takes the list of names literally
  envsubst "$vars" <"$here/conf/$template" >"$target"
}

need_root() {
  [ "$(id -u)" = 0 ] || { echo "run as root" >&2; exit 1; }
}
