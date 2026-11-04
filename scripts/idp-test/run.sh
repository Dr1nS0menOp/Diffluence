#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# Proof against real software (phase 2 exit): Authentik provisions users and groups
# through SCIM, then a real browser logs in through Authentik's own screens.
# Uses its own compose project and removes it afterwards. Run from WSL or Linux:
#   docker build -t diffluence:local . && bash scripts/idp-test/run.sh
set -euo pipefail
cd "$(dirname "$0")"
trap 'docker compose down -v >/dev/null 2>&1' EXIT
docker compose up -d >/dev/null 2>&1
for _ in $(seq 1 100); do
  curl -sf -o /dev/null http://localhost:19000/-/health/ready/ && break
  sleep 3
done
for _ in $(seq 1 60); do
  curl -sf -o /dev/null -H 'Authorization: Bearer bootstrap-token-for-testing-0123456789' http://localhost:19000/api/v3/admin/version/ && break
  sleep 3
done
python3 scim_test.py
python3 oidc_setup.py
sleep 10 # Authentik provisions the new users through SCIM in the background
docker run --rm --network container:dfl-idp-diffluence-1 \
  -v diffluence-dev_web_modules:/work/web/node_modules \
  -v "$PWD:/t" -w /t mcr.microsoft.com/playwright:v1.64.1-noble \
  node oidc_browser.cjs alice carol
echo "identity provider test passed"
