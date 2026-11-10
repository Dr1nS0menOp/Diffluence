#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# Sets up the app machine of the stress test the way the install guide says, plus two
# operator choices the guide leaves open: PostgreSQL settings for a 32 GB machine, and
# pg_stat_statements so the test can see which queries cost most (compose.stress.yaml).
# Usage on a fresh Ubuntu 24.04: bash app-vm.sh <public url> <git ref>
set -eu
public_url=$1
ref=${2:-main}

sudo apt-get update -q
sudo DEBIAN_FRONTEND=noninteractive apt-get install -yq docker.io docker-compose-v2 git sysstat
sudo usermod -aG docker "$USER"

if [ ! -d Diffluence ]; then
  git clone -q https://github.com/Dr1nS0menOp/Diffluence.git
fi
cd Diffluence
git fetch -q origin && git checkout -q "$ref"
git log --oneline -1

sh scripts/setup-secrets.sh
cat > .env <<ENV
DIFFLUENCE_PUBLIC_URL=$public_url
DIFFLUENCE_BIND=0.0.0.0
DIFFLUENCE_METRICS_TOKEN=$(head -c 24 /dev/urandom | base64 | tr -d '/+=')
DIFFLUENCE_EXAMPLE_SPACE=false
ENV
cp ../compose.stress.yaml compose.override.yaml
sudo docker compose up -d --build --wait
sudo docker compose ps
