#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
"""Opens many connections that send a request one header line at a time, slowly, the
way a slow or hostile client would, and reports how many the server closed and when.
Usage: slow-clients.py <host> <port> <connections> <seconds>"""
import socket
import sys
import time


def main():
    host, port, count, seconds = sys.argv[1], int(sys.argv[2]), int(sys.argv[3]), int(sys.argv[4])
    socks, refused = [], 0
    for _ in range(count):
        try:
            s = socket.create_connection((host, port), timeout=5)
            s.sendall(f"GET /api/v1/spaces HTTP/1.1\r\nHost: {host}\r\n".encode())
            socks.append(s)
        except OSError:
            refused += 1
    print(f"opened {len(socks)}, refused {refused}", flush=True)
    start = time.time()
    while time.time() - start < seconds and socks:
        alive = []
        for s in socks:
            try:
                s.sendall(b"X-Slow: 1\r\n")
                alive.append(s)
            except OSError:
                s.close()
        socks = alive
        print(f"{time.time() - start:5.0f}s still open {len(socks)}", flush=True)
        time.sleep(5)
    for s in socks:
        s.close()


if __name__ == "__main__":
    main()
