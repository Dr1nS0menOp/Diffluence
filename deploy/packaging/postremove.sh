#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
# After the .deb or .rpm removes Kenwerk (spec 057). The data, the configuration and the
# kenwerk user stay: removing a package must never remove a wiki. The guide says how to
# remove them by hand.
set -e

if [ -d /run/systemd/system ] && command -v systemctl >/dev/null 2>&1; then
  systemctl daemon-reload || true
fi
exit 0
