#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
"""Creates many spaces at once as an administrator and times a plain read meanwhile.
In the first stress test 24 parallel creations froze the instance. Usage on the load machine:
  python3 freeze.py --base http://app:8080 --admin-email a@example.test --count 24
The admin password is read from ~/admin.password. Uses the Client of seed.py."""
import argparse
import os
import threading
import time

from seed import Client


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--base", required=True)
    ap.add_argument("--admin-email", required=True)
    ap.add_argument("--count", type=int, default=24)
    a = ap.parse_args()
    password = open(os.path.expanduser("~/admin.password")).read().strip()
    stamp = int(time.time())
    results = []
    lock = threading.Lock()

    # One login for all: 24 logins at once would meet the login limit before any creation.
    # The client keeps one connection per thread, so the creations still run in parallel.
    admin = Client(a.base, a.admin_email, password)

    def create(n):
        start = time.monotonic()
        status, _ = admin.call("POST", "/api/v1/spaces", {"key": f"F{stamp % 10000:04d}{n:02d}", "name": f"Freeze test {n}"})
        with lock:
            results.append((status, time.monotonic() - start))

    reader = admin
    reads = []
    done = threading.Event()

    def read_loop():
        while not done.is_set():
            start = time.monotonic()
            status, _ = reader.call("GET", "/api/v1/spaces")
            reads.append((status, time.monotonic() - start))
            time.sleep(0.2)

    t_read = threading.Thread(target=read_loop)
    t_read.start()
    threads = [threading.Thread(target=create, args=(n,)) for n in range(a.count)]
    began = time.monotonic()
    for t in threads:
        t.start()
    for t in threads:
        t.join(timeout=120)
    total = time.monotonic() - began
    done.set()
    t_read.join()
    times = sorted(d for _, d in results)
    statuses = sorted({s for s, _ in results})
    print(f"creations: {len(results)}/{a.count} answered in {total:.1f}s, statuses {statuses}, "
          f"slowest {times[-1] if times else 0:.2f}s, median {times[len(times) // 2] if times else 0:.2f}s")
    rt = sorted(d for _, d in reads)
    print(f"reads meanwhile: {len(rt)}, slowest {rt[-1] if rt else 0:.2f}s, median {rt[len(rt) // 2] if rt else 0:.2f}s")


if __name__ == "__main__":
    main()
