#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
# After the .deb or .rpm installs or upgrades Kenwerk (spec 057). Debian passes
# "configure"; RPM passes 1 on an install and 2 on an upgrade.
set -e

if ! getent group kenwerk >/dev/null 2>&1; then
  groupadd --system kenwerk
fi
if ! getent passwd kenwerk >/dev/null 2>&1; then
  # Debian has /usr/sbin/nologin, a minimal Fedora only /sbin/nologin or neither.
  shell=$(command -v nologin 2>/dev/null || true)
  [ -n "$shell" ] || for shell in /usr/sbin/nologin /sbin/nologin /bin/false; do [ -x "$shell" ] && break; done
  useradd --system --gid kenwerk --home-dir /var/lib/kenwerk --no-create-home \
    --shell "$shell" --comment "Kenwerk wiki" kenwerk
fi

# The data folder holds master.key: only the service may read it.
install -d -m 0700 -o kenwerk -g kenwerk /var/lib/kenwerk
install -d -m 0750 -o root -g kenwerk /etc/kenwerk

if [ -d /run/systemd/system ] && command -v systemctl >/dev/null 2>&1; then
  systemctl daemon-reload
  # An upgrade restarts a service that runs, as Debian packages do; the checks before an
  # upgrade then decide whether it may migrate (spec 054). A first install never starts it:
  # there is no database address yet.
  systemctl try-restart kenwerk.service
fi

if [ ! -e /etc/kenwerk/kenwerk.env ] && [ ! -e /etc/kenwerk/kenwerk.yaml ]; then
  cat <<'NEXT'
Kenwerk is installed but not started: it needs a database and a public address first.
See "Packages" in docs/operations/install.md, or /usr/share/doc/kenwerk/, then:
  systemctl enable --now kenwerk
NEXT
fi
exit 0
