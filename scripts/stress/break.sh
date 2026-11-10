#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# The breaking tests of the stress test, run on the app machine while load.js runs the
# "level" scenario from the load machine. Each prints what it did and when.
# Usage: bash break.sh pg-restart | app-kill | full-disk | full-disk-undo | audit-verify
set -u
cd ~/Diffluence || exit 1
dc() { sudo docker compose "$@" < /dev/null; }
case $1 in
  pg-restart)
    echo "$(date +%T) restarting PostgreSQL"
    dc restart postgres
    echo "$(date +%T) PostgreSQL is back"
    ;;
  app-kill)
    # The process is killed, as a crash would: `docker kill` counts as a deliberate stop,
    # which the restart policy leaves stopped.
    echo "$(date +%T) killing the app's process with SIGKILL"
    sudo kill -9 "$(sudo docker inspect -f '{{.State.Pid}}' diffluence-diffluence-1)"
    sleep 1
    for i in $(seq 1 60); do
      [ "$(curl -s -m 2 -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/readyz)" = 200 ] && break
      sleep 1
    done
    echo "$(date +%T) ready again after about ${i}s"
    ;;
  full-disk)
    # Leaves about 5 MB on the disk that holds PostgreSQL and the files.
    free=$(df --output=avail -B1M /var/lib/docker | tail -1)
    sudo fallocate -l "$((free - 5))M" /var/lib/docker/stress-fill
    echo "$(date +%T) disk filled: $(df -h --output=avail /var/lib/docker | tail -1) left"
    ;;
  full-disk-undo)
    sudo rm -f /var/lib/docker/stress-fill
    echo "$(date +%T) disk freed: $(df -h --output=avail /var/lib/docker | tail -1) left"
    ;;
  audit-verify)
    # The container already reads the owner's address from its secret file.
    sudo docker compose exec -T diffluence diffluence audit verify < /dev/null 2>&1 | tail -5
    ;;
esac
