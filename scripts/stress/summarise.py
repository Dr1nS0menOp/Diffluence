#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
"""Prints the k6 summary of one run as a table per route: requests, p50, p95, p99, max,
failed share and busy (503) answers.  Usage: summarise.py summary-mix.json"""
import json
import re
import sys


def main():
    data = json.load(open(sys.argv[1]))
    m = data["metrics"]
    rows = []
    for name, metric in m.items():
        hit = re.fullmatch(r"http_req_duration\{route:(\w+)\}", name)
        if hit:
            v = metric["values"]
            rows.append((hit.group(1), v))
    reqs = m.get("http_reqs", {}).get("values", {})
    dur = data.get("state", {}).get("testRunDurationMs", 0) / 1000
    print(f"run {dur:.0f}s, requests {reqs.get('count', 0)}, {reqs.get('rate', 0):.1f}/s, "
          f"failed {m.get('http_req_failed', {}).get('values', {}).get('rate', 0) * 100:.2f}%, "
          f"busy 503 {m.get('busy_503', {}).get('values', {}).get('count', 0)}, "
          f"failed logins {m.get('failed_logins', {}).get('values', {}).get('count', 0)}, "
          f"max VUs {m.get('vus_max', {}).get('values', {}).get('max', 0)}")
    print(f"{'route':<12} {'p50 ms':>8} {'p95 ms':>8} {'p99 ms':>8} {'max ms':>9}")
    for route, v in sorted(rows):
        print(f"{route:<12} {v.get('p(50)', v.get('med', 0)):>8.0f} {v.get('p(95)', 0):>8.0f} "
              f"{v.get('p(99)', 0):>8.0f} {v.get('max', 0):>9.0f}")
    checks = {k: v for k, v in m.items() if k == "checks"}
    if checks:
        c = checks["checks"]["values"]
        print(f"checks passed {c.get('rate', 0) * 100:.2f}%")


if __name__ == "__main__":
    main()
