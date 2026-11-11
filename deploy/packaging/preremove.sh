#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
# Before the .deb or .rpm removes Diffluence (spec 057). Debian passes "remove" for a
# removal and "upgrade" for an upgrade; RPM passes 0 for a removal and 1 for an upgrade.
# Only a real removal stops the service: an upgrade restarts it afterwards instead.
set -e

case "$1" in
  remove | 0)
    if [ -d /run/systemd/system ] && command -v systemctl >/dev/null 2>&1; then
      systemctl disable --now diffluence.service || true
    fi
    ;;
esac
exit 0
