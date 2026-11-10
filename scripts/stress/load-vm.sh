#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# Sets up the load machine of the stress test: k6 from its official release, nothing else.
set -eu
tag=$(curl -sSf https://api.github.com/repos/grafana/k6/releases/latest | python3 -c 'import json,sys; print(json.load(sys.stdin)["tag_name"])')
curl -sSfL "https://github.com/grafana/k6/releases/download/$tag/k6-$tag-linux-amd64.tar.gz" | tar xz
sudo install "k6-$tag-linux-amd64/k6" /usr/local/bin/k6
k6 version
# 5,000 simulated people each keep a connection open.
echo '* soft nofile 200000' | sudo tee /etc/security/limits.d/k6.conf >/dev/null
echo '* hard nofile 200000' | sudo tee -a /etc/security/limits.d/k6.conf >/dev/null
sudo sysctl -qw net.ipv4.ip_local_port_range="1024 65000" net.ipv4.tcp_tw_reuse=1
