#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
"""Publishes one very large page, just under the 5 MiB limit of a document, then has 20
people read it at once, and reports how long each step took.
Usage: SEED_ADMIN_PASSWORD=... huge-page.py <base> <admin email> <seeded.json>"""
import concurrent.futures
import json
import os
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from seed import Client  # noqa: E402


def main():
    base, email, seeded = sys.argv[1], sys.argv[2], sys.argv[3]
    admin = Client(base, email, os.environ["SEED_ADMIN_PASSWORD"])
    seed = json.load(open(seeded))
    space = seed["writable_spaces"][1]
    page = admin.ok("POST", "/api/v1/pages", {"space_id": space, "title": "A very large page"})["id"]
    paragraph = "Lorem ipsum dolor sit amet, consectetur adipiscing elit. " * 18
    # Just under the limit of 5 MiB on a document.
    content = [{"type": "paragraph", "content": [{"type": "text", "text": paragraph}]} for _ in range(4500)]
    doc = {"type": "doc", "content": content}
    body = json.dumps({"title": "A very large page", "document": doc, "base_version": 0})
    print(f"document of {len(body) / 1e6:.1f} MB")
    t = time.time()
    status, text = admin.call("POST", f"/api/v1/pages/{page}/publish", raw=body.encode())
    print(f"publish: {status} in {time.time() - t:.1f}s {text[:150] if status >= 300 else ''}")
    password = open(seeded + ".password").read().strip()
    readers = [Client(base, f"person{i:05d}@example.test", password) for i in range(20)]

    def read(c):
        t0 = time.time()
        s, b = c.call("GET", f"/api/v1/pages/{page}")
        return s, round(time.time() - t0, 2), len(b)
    with concurrent.futures.ThreadPoolExecutor(20) as pool:
        results = list(pool.map(read, readers))
    print("20 readers at once (status, seconds, bytes):", sorted(results))
    t = time.time()
    s, b = readers[0].call("GET", f"/api/v1/search?q=consectetur")
    print(f"search for a word of that page: {s} in {time.time() - t:.2f}s")


if __name__ == "__main__":
    main()
