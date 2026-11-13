#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
"""The write ledger of spec 062: proves that no acknowledged write is lost.

"write" adds comments with a unique mark to one page through the app's address, one thread
per API token, each paced below the per-person and per-token limits. It records every mark
the app acknowledged (HTTP 2xx). A refusal (409, 429, 503) or no answer is not an
acknowledgement and is only counted.

"verify" reads the page's comments back through each api node and reports any acknowledged
mark that is missing (a lost write), present twice (a doubled write), or seen on one node
but not the other (two truths). Standard library only.
"""

import argparse
import json
import sys
import threading
import time
import urllib.error
import urllib.request
import uuid


def call(base, method, path, token, body=None, timeout=10):
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(base + path, data=data, method=method)
    req.add_header("Content-Type", "application/json")
    req.add_header("Authorization", "Bearer " + token)
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        return resp.status, resp.read()


def doc(text):
    return {"type": "doc", "content": [{"type": "paragraph", "content": [{"type": "text", "text": text}]}]}


def text_of(node):
    if isinstance(node, dict):
        return node.get("text", "") + "".join(text_of(c) for c in node.get("content", []))
    return ""


def write(args):
    with open(args.tokens, encoding="utf-8") as f:
        tokens = [t.strip() for t in f if t.strip()]
    stop = time.time() + args.seconds
    lock = threading.Lock()
    acked, refused, unanswered = [], [0], [0]

    def worker(token):
        while time.time() < stop:
            mark = uuid.uuid4().hex
            try:
                status, _ = call(args.base, "POST", f"/api/v1/pages/{args.page}/comments", token,
                                 {"body": doc(f"ledger {mark}")})
                if 200 <= status < 300:
                    with lock:
                        acked.append({"mark": mark, "at": time.time()})
            except urllib.error.HTTPError:
                with lock:
                    refused[0] += 1
            except (urllib.error.URLError, OSError):
                with lock:
                    unanswered[0] += 1
            time.sleep(args.pause)

    threads = [threading.Thread(target=worker, args=(t,)) for t in tokens]
    for t in threads:
        t.start()
    for t in threads:
        t.join()
    with open(args.ledger, "w", encoding="utf-8") as out:
        json.dump({"page": args.page, "acked": acked, "refused": refused[0], "unanswered": unanswered[0]}, out)
    print(f"acknowledged {len(acked)}, refused {refused[0]}, unanswered {unanswered[0]}")


def marks_on(base, page, token):
    found, offset = {}, 0
    while True:
        _, body = call(base, "GET", f"/api/v1/pages/{page}/comments?limit=100&offset={offset}", token, timeout=60)
        threads = json.loads(body).get("threads", [])
        for c in threads:
            text = text_of(c.get("body"))
            if text.startswith("ledger "):
                mark = text.split(" ", 1)[1].strip()
                found[mark] = found.get(mark, 0) + 1
        if len(threads) < 100:
            return found
        offset += 100


def verify(args):
    with open(args.ledger, encoding="utf-8") as f:
        ledger = json.load(f)
    acked = {a["mark"] for a in ledger["acked"]}
    views = {node: marks_on(node, ledger["page"], args.token) for node in args.nodes}
    problems = 0
    for node, found in views.items():
        lost = acked - found.keys()
        doubled = [m for m, n in found.items() if n > 1]
        print(f"{node}: {len(found)} found, {len(lost)} acknowledged but missing, {len(doubled)} doubled")
        problems += len(lost) + len(doubled)
    first = next(iter(views.values()))
    for node, found in views.items():
        if found.keys() != first.keys():
            print(f"{node} shows a different set than {args.nodes[0]}: two truths")
            problems += 1
    sys.exit(1 if problems else 0)


def main():
    p = argparse.ArgumentParser(description=__doc__)
    sub = p.add_subparsers(dest="cmd", required=True)
    w = sub.add_parser("write")
    w.add_argument("--base", required=True, help="the app's address, through HAProxy")
    w.add_argument("--page", required=True)
    w.add_argument("--tokens", required=True, help="a file with one API token per line, each of another person")
    w.add_argument("--seconds", type=int, default=300)
    w.add_argument("--pause", type=float, default=7.0, help="seconds between writes of one token")
    w.add_argument("--ledger", default="ledger.json")
    v = sub.add_parser("verify")
    v.add_argument("--nodes", nargs="+", required=True, help="the address of each api node")
    v.add_argument("--token", required=True)
    v.add_argument("--ledger", default="ledger.json")
    args = p.parse_args()
    if args.cmd == "write":
        write(args)
    else:
        verify(args)


if __name__ == "__main__":
    main()
