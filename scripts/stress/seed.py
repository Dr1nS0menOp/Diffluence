#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
"""Fills a test instance with the shape of an organisation of 5,000 people, through the
public API only, so every row has its audit event and every file is encrypted as in use.

Run it against a test instance, never a live one:
  python3 seed.py --base http://app:8080 --admin-email a@example.test --out seeded.json
The admin password is read from the environment variable SEED_ADMIN_PASSWORD.

Sizes (why these numbers: docs/reports/2026-11-10-stress-test.md, "The data"):
5,000 people, 120 groups, 300 spaces, 60,000 pages three levels deep, about 30% with
more versions, 30,000 comments, 12,000 files and 25,000 labels.
"""
import argparse
import concurrent.futures
import http.client
import http.cookies
import json
import os
import random
import secrets
import sys
import threading
import time
import urllib.parse


def build_vocabulary(rng):
    """A made-up language with a Zipf-like spread, so some words are in most pages and
    most words are rare, as in real text."""
    syllables = ["ka", "lo", "mi", "ne", "ru", "ta", "vo", "se", "di", "pa", "ze", "fo",
                 "gri", "bel", "tor", "san", "wen", "dru", "mol", "fin"]
    words = set()
    while len(words) < 20000:
        words.add("".join(rng.choice(syllables) for _ in range(rng.randint(2, 4))))
    words = sorted(words)
    rng.shuffle(words)
    weights = [1.0 / (rank + 1) for rank in range(len(words))]
    return words, weights


class Client:
    """One logged-in person. Thread-safe enough: one connection per thread."""

    def __init__(self, base, email, password):
        self.base = urllib.parse.urlparse(base)
        self.origin = base.rstrip("/")
        self.local = threading.local()
        self.cookie = ""
        status, body = self.call("POST", "/api/v1/auth/login", {"email": email, "password": password})
        if status != 200:
            raise RuntimeError(f"login of {email} answered {status}: {body[:200]}")

    def conn(self):
        if not hasattr(self.local, "c"):
            self.local.c = http.client.HTTPConnection(self.base.hostname, self.base.port or 80, timeout=600)
        return self.local.c

    def call(self, method, path, body=None, raw=None, content_type="application/json"):
        headers = {"Origin": self.origin}
        if self.cookie:
            headers["Cookie"] = self.cookie
        data = None
        if raw is not None:
            data = raw
            headers["Content-Type"] = content_type
        elif body is not None:
            data = json.dumps(body).encode()
            headers["Content-Type"] = "application/json"
        for attempt in range(6):
            try:
                c = self.conn()
                c.request(method, path, body=data, headers=headers)
                r = c.getresponse()
                text = r.read().decode("utf-8", "replace")
                if r.status == 503:
                    time.sleep(1 + attempt)
                    continue
                for value in r.headers.get_all("Set-Cookie") or []:
                    jar = http.cookies.SimpleCookie()
                    jar.load(value)
                    for k, m in jar.items():
                        self.cookie = f"{k}={m.value}"
                return r.status, text
            except (http.client.HTTPException, OSError):
                self.local.c = http.client.HTTPConnection(self.base.hostname, self.base.port or 80, timeout=600)
                time.sleep(1 + attempt)
        return 0, "gave up"

    def ok(self, method, path, body=None, want=(200, 201, 204), **kw):
        status, text = self.call(method, path, body, **kw)
        if status not in want:
            raise RuntimeError(f"{method} {path} answered {status}: {text[:300]}")
        return json.loads(text) if text.strip().startswith("{") else {}


def doc(paragraphs):
    content = []
    for i, p in enumerate(paragraphs):
        if i % 4 == 0:
            content.append({"type": "heading", "attrs": {"level": 2},
                            "content": [{"type": "text", "text": " ".join(p.split()[:5])}]})
        content.append({"type": "paragraph", "content": [{"type": "text", "text": p}]})
    return {"type": "doc", "content": content}


def text_of(rng, words, weights, n):
    picked = rng.choices(words, weights=weights, k=n)
    sentences, out = [], []
    for w in picked:
        out.append(w)
        if len(out) >= rng.randint(8, 18):
            sentences.append(" ".join(out).capitalize() + ".")
            out = []
    if out:
        sentences.append(" ".join(out).capitalize() + ".")
    paragraphs, chunk = [], []
    for s in sentences:
        chunk.append(s)
        if len(chunk) >= 4:
            paragraphs.append(" ".join(chunk))
            chunk = []
    if chunk:
        paragraphs.append(" ".join(chunk))
    return paragraphs


VIEWER = ["space.view", "space.export", "comment.add", "comment.delete_own"]
EDITOR = VIEWER + ["page.create", "page.edit", "page.archive", "page.delete_own", "attachment.add",
                   "attachment.delete_own", "page.move_out", "post.create", "database.create"]


def run(pool, fn, items, label):
    done, failed, start = 0, 0, time.time()
    results = []
    futures = [pool.submit(fn, item) for item in items]
    for f in concurrent.futures.as_completed(futures):
        try:
            results.append(f.result())
        except Exception as e:  # one failed item is counted and reported, the run goes on
            failed += 1
            if failed <= 5:
                print(f"  {label}: {e}", file=sys.stderr)
        done += 1
        if done % 2000 == 0:
            print(f"  {label}: {done}/{len(items)} in {time.time() - start:.0f}s", flush=True)
    print(f"{label}: {len(results)} made, {failed} failed, {time.time() - start:.0f}s", flush=True)
    return results


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--base", required=True)
    ap.add_argument("--admin-email", required=True)
    ap.add_argument("--out", required=True)
    ap.add_argument("--people", type=int, default=5000)
    ap.add_argument("--groups", type=int, default=120)
    ap.add_argument("--spaces", type=int, default=300)
    ap.add_argument("--pages", type=int, default=60000)
    ap.add_argument("--comments", type=int, default=30000)
    ap.add_argument("--files", type=int, default=12000)
    ap.add_argument("--labels", type=int, default=25000)
    ap.add_argument("--workers", type=int, default=16)
    ap.add_argument("--commenters", type=int, default=300)
    ap.add_argument("--resume", action="store_true",
                    help="keep the people, groups, spaces and pages of an earlier run and add what is missing")
    a = ap.parse_args()
    rng = random.Random(20261110)
    words, weights = build_vocabulary(rng)
    admin = Client(a.base, a.admin_email, os.environ["SEED_ADMIN_PASSWORD"])
    pool = concurrent.futures.ThreadPoolExecutor(a.workers)
    earlier = {}

    if a.resume:
        with open(a.out) as f:
            earlier = json.load(f)
        with open(a.out + ".password") as f:
            password = f.read().strip()
        people = {i: None for i in range(earlier["people"])}
        listed = admin.ok("GET", "/api/v1/admin/groups?limit=1000")["groups"]
        groups = {int(g["name"][5:]): g["id"] for g in listed if g["name"].startswith("Team ")}
        everyone = next(g["id"] for g in listed if g["name"] == "All staff")
    else:
        password = "Stress-" + secrets.token_urlsafe(18)

        def make_person(i):
            u = admin.ok("POST", "/api/v1/admin/users", {
                "email": f"person{i:05d}@example.test", "display_name": f"Person {i:05d}", "password": password})
            return (i, u["id"])
        people = dict(run(pool, make_person, range(a.people), "people"))

        groups = run(pool, lambda g: (g, admin.ok("POST", "/api/v1/admin/groups", {"name": f"Team {g:03d}"})["id"]),
                     range(a.groups), "groups")
        groups = dict(groups)
        everyone = admin.ok("POST", "/api/v1/admin/groups", {"name": "All staff"})["id"]
        memberships = []
        for i in people:
            memberships.append((everyone, people[i]))
            for g in rng.sample(range(a.groups), rng.choice([1, 1, 2, 3])):
                memberships.append((groups[g], people[i]))
        run(pool, lambda m: admin.ok("PUT", f"/api/v1/admin/groups/{m[0]}/members/{m[1]}"), memberships, "memberships")

    existing = {}
    if a.resume:
        for sp in admin.ok("GET", "/api/v1/admin/spaces")["spaces"]:
            if sp["key"].startswith("S") and sp["key"][1:].isdigit():
                existing[int(sp["key"][1:])] = sp["id"]

    def make_space(s):
        if s in existing:
            sid = existing[s]
        else:
            sp = admin.ok("POST", "/api/v1/spaces", {"key": f"S{s:04d}", "name": f"Space {s:04d} " + " ".join(rng.sample(words[:500], 2))})
            sid = sp["id"]
        # The grants are set again on a resumed space: one may have failed the first time.
        # Half the spaces are open to all staff to read, and one in ten to write, so the
        # load test can give every simulated person pages they may see and change.
        if s % 10 == 0:
            admin.ok("PUT", f"/api/v1/spaces/{sid}/grants", {"subject": {"type": "group", "id": everyone}, "permissions": EDITOR})
        elif s % 2 == 0:
            admin.ok("PUT", f"/api/v1/spaces/{sid}/grants", {"subject": {"type": "group", "id": everyone}, "permissions": VIEWER})
        for g in rng.sample(range(a.groups), 3):
            admin.ok("PUT", f"/api/v1/spaces/{sid}/grants", {"subject": {"type": "group", "id": groups[g]}, "permissions": VIEWER})
        g = rng.randrange(a.groups)
        admin.ok("PUT", f"/api/v1/spaces/{sid}/grants", {"subject": {"type": "group", "id": groups[g]}, "permissions": EDITOR})
        return (s, sid)
    # Spaces are made a few at a time, as administrators make them.
    spaces = dict(run(concurrent.futures.ThreadPoolExecutor(4), make_space, range(a.spaces), "spaces"))

    # Space sizes follow a long tail: a few large handbooks, many small team spaces.
    sizes = [1.0 / (k + 1) ** 0.8 for k in range(a.spaces)]
    total = sum(sizes)
    plan = []
    for s in range(a.spaces):
        n = max(5, int(a.pages * sizes[s] / total))
        for j in range(n):
            plan.append((s, j))
    plan = plan[: a.pages]
    if a.resume:
        # Pages of the earlier run stay; only the missing number is added, spread the same way.
        missing = a.pages - len(earlier.get("pages", []))
        rng.shuffle(plan)
        plan = sorted(plan[: max(0, missing)])
    parents = {}
    lock = threading.Lock()

    def make_page(item):
        s, j = item
        parent = None
        with lock:
            level = parents.setdefault(s, [[], []])
            if j >= 10 and level[0] and rng.random() < 0.7:
                parent = rng.choice(level[1] if level[1] and rng.random() < 0.5 else level[0])
        title = " ".join(w.capitalize() for w in rng.choices(words, weights=weights, k=rng.randint(2, 6)))
        body = {"space_id": spaces[s], "title": title}
        if parent:
            body["parent_id"] = parent
        pid = admin.ok("POST", "/api/v1/pages", body)["id"]
        paragraphs = text_of(rng, words, weights, rng.randint(150, 900))
        admin.ok("POST", f"/api/v1/pages/{pid}/publish", {"title": title, "document": doc(paragraphs), "base_version": 0})
        version = 1
        if rng.random() < 0.3:
            for _ in range(rng.randint(1, 5)):
                paragraphs.append(" ".join(text_of(rng, words, weights, 60)))
                admin.ok("POST", f"/api/v1/pages/{pid}/publish", {"title": title, "document": doc(paragraphs), "base_version": version})
                version += 1
        with lock:
            (level[0] if parent is None else level[1]).append(pid)
        return (s, pid)
    pages = run(pool, make_page, plan, "pages")
    page_ids = earlier.get("pages", []) + [p for _, p in pages]
    readable = earlier.get("readable_pages", []) + [p for s, p in pages if s % 2 == 0]
    writable = earlier.get("writable_pages", []) + [p for s, p in pages if s % 10 == 0]

    # Comments come from many people, on pages all staff can read: one person may only
    # write so many comments a minute, as in real use.
    writers = run(pool, lambda i: Client(a.base, f"person{i:05d}@example.test", password),
                  range(min(a.commenters, len(people))), "commenters")

    def make_comment(k):
        who = writers[k % len(writers)]
        pid = rng.choice(readable)
        # A comment holds paragraphs only, no headings.
        text = " ".join(text_of(rng, words, weights, 25))
        for attempt in range(8):
            status, body = who.call("POST", f"/api/v1/pages/{pid}/comments", {"body": {"type": "doc", "content": [
                {"type": "paragraph", "content": [{"type": "text", "text": text}]}]}})
            if status in (200, 201):
                return
            if status == 409 and "too_many_comments" in body:
                time.sleep(15)
                continue
            raise RuntimeError(f"comment answered {status}: {body[:200]}")
        raise RuntimeError("comment kept hitting the limit per minute")
    run(pool, make_comment, range(a.comments), "comments")

    def make_file(k):
        pid = rng.choice(page_ids)
        size = int(rng.lognormvariate(10, 1.5)) % (2 << 20) + 200
        name = f"{rng.choice(words)}-{k}.{rng.choice(['pdf', 'txt', 'png', 'docx', 'csv'])}"
        admin.ok("POST", f"/api/v1/pages/{pid}/attachments?filename={urllib.parse.quote(name)}",
                 raw=os.urandom(size), content_type="application/octet-stream")
    label_words = words[:300]

    def make_label(_):
        pid = rng.choice(page_ids)
        admin.call("POST", f"/api/v1/pages/{pid}/labels", {"label": rng.choice(label_words)})
    # A resumed run already has its files and labels.
    if not a.resume:
        run(pool, make_file, range(a.files), "files")
        run(pool, make_label, range(a.labels), "labels")

    common = words[:5]
    rare = words[5000:5020]
    with open(a.out, "w") as f:
        json.dump({"password_env": "the seeded people share one password, kept only on the load machine",
                   "people": len(people), "spaces": list(spaces.values()), "pages": page_ids,
                   "readable_spaces": [spaces[s] for s in spaces if s % 2 == 0],
                   "writable_spaces": [spaces[s] for s in spaces if s % 10 == 0],
                   "readable_pages": readable, "writable_pages": writable,
                   "common_words": common, "rare_words": rare, "mid_words": words[200:240]}, f)
    with open(a.out + ".password", "w") as f:
        f.write(password)
    os.chmod(a.out + ".password", 0o600)
    print("done")


if __name__ == "__main__":
    main()
