#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# Samples the app machine every 5 seconds while a test runs: CPU and memory per container,
# the machine's CPU and disk, database connections by state, and /readyz.
# Usage on the app machine: bash monitor.sh <out file> &   (stop it with kill)
out=$1
cd ~/Diffluence || exit 1
echo "time,container,cpu,mem" > "$out.containers"
echo "time,us,sy,wa,id,disk_util" > "$out.machine"
echo "time,active,idle,idle_in_tx,waiting" > "$out.db"
echo "time,readyz" > "$out.ready"
while true; do
  t=$(date +%s)
  sudo docker stats --no-stream --format "$t,{{.Name}},{{.CPUPerc}},{{.MemUsage}}" >> "$out.containers"
  vmstat 1 2 | tail -1 | awk -v t="$t" '{print t","$13","$14","$16","$15}' | tr -d '\n' >> "$out.machine"
  iostat -dx 1 2 | awk '/^(sda|vda|sdb)/{u=$NF} END{print ","u}' >> "$out.machine"
  sudo docker compose exec -T postgres psql -U diffluence_owner -d diffluence -tAc \
    "SELECT $t, count(*) FILTER (WHERE state='active'), count(*) FILTER (WHERE state='idle'),
            count(*) FILTER (WHERE state LIKE 'idle in%'), count(*) FILTER (WHERE wait_event_type='Lock')
     FROM pg_stat_activity WHERE datname='diffluence'" | tr '|' ',' >> "$out.db"
  echo "$t,$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 http://127.0.0.1:8080/readyz)" >> "$out.ready"
  # How far the audit sealer is behind (spec 052); needs the metrics token from .env.
  token=$(grep -E '^DIFFLUENCE_METRICS_TOKEN=' .env | cut -d= -f2)
  lag=$(curl -s --max-time 5 -H "Authorization: Bearer $token" http://127.0.0.1:8080/metrics |
    awk '/^diffluence_audit_sealing_lag_seconds /{print $2}')
  echo "$t,${lag:-}" >> "$out.seal"
  sleep 3
done
