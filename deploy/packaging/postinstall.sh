#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
# After the .deb or .rpm installs or upgrades Diffluence (spec 057). Debian passes
# "configure"; RPM passes 1 on an install and 2 on an upgrade.
set -e

if ! getent group diffluence >/dev/null 2>&1; then
  groupadd --system diffluence
fi
if ! getent passwd diffluence >/dev/null 2>&1; then
  # Debian has /usr/sbin/nologin, a minimal Fedora only /sbin/nologin or neither.
  shell=$(command -v nologin 2>/dev/null || true)
  [ -n "$shell" ] || for shell in /usr/sbin/nologin /sbin/nologin /bin/false; do [ -x "$shell" ] && break; done
  useradd --system --gid diffluence --home-dir /var/lib/diffluence --no-create-home \
    --shell "$shell" --comment "Diffluence wiki" diffluence
fi

# The data folder holds master.key: only the service may read it.
install -d -m 0700 -o diffluence -g diffluence /var/lib/diffluence
install -d -m 0750 -o root -g diffluence /etc/diffluence

if [ -d /run/systemd/system ] && command -v systemctl >/dev/null 2>&1; then
  systemctl daemon-reload
  # An upgrade restarts a service that runs, as Debian packages do; the checks before an
  # upgrade then decide whether it may migrate (spec 054). A first install never starts it:
  # there is no database address yet.
  systemctl try-restart diffluence.service
fi

if [ ! -e /etc/diffluence/diffluence.env ] && [ ! -e /etc/diffluence/diffluence.yaml ]; then
  cat <<'NEXT'
Diffluence is installed but not started: it needs a database and a public address first.
See "Packages" in docs/operations/install.md, or /usr/share/doc/diffluence/, then:
  systemctl enable --now diffluence
NEXT
fi
exit 0
