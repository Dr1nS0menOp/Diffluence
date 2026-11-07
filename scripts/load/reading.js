// SPDX-License-Identifier: Apache-2.0
// k6 load test: the real pattern of a reader, with the polling the frontend does.
// k6 is a tool an operator installs, not a dependency of Diffluence.
//   k6 run -e BASE_URL=http://localhost:8080 -e EMAIL=you@example.com \
//     -e PASSWORD=... -e SPACE=<space id> -e PAGE=<page id> scripts/load/reading.js
// The targets per query are the ones of blueprint 04 (ARC-199, ARC-200). A run at full
// size needs a space of 50,000 pages: docs/operations/limits.md has the results.
import http from "k6/http";
import { check, sleep } from "k6";

const base = __ENV.BASE_URL;
const space = __ENV.SPACE;
const page = __ENV.PAGE;

export const options = {
  stages: [
    { duration: "1m", target: Number(__ENV.READERS || 50) },
    { duration: "3m", target: Number(__ENV.READERS || 50) },
    { duration: "30s", target: 0 },
  ],
  thresholds: {
    http_req_failed: ["rate<0.01"],
    "http_req_duration{query:tree}": ["p(95)<500"],
    "http_req_duration{query:search}": ["p(95)<500"],
    "http_req_duration{query:feed}": ["p(95)<500"],
    "http_req_duration{query:directory}": ["p(95)<500"],
    "http_req_duration{query:page}": ["p(95)<300"],
    "http_req_duration{query:poll}": ["p(95)<100"],
  },
};

function get(path, query) {
  const response = http.get(`${base}${path}`, { tags: { query } });
  check(response, { "answered": (r) => r.status === 200 });
}

export function setup() {
  if (!base || !space || !page) {
    throw new Error("BASE_URL, SPACE and PAGE are needed");
  }
}

export default function () {
  if (__ITER === 0) {
    const login = http.post(`${base}/api/v1/auth/login`, JSON.stringify({ email: __ENV.EMAIL, password: __ENV.PASSWORD }), {
      headers: { "Content-Type": "application/json", Origin: base },
    });
    check(login, { "logged in": (r) => r.status === 200 });
  }
  get(`/api/v1/spaces/${space}/tree`, "tree");
  get(`/api/v1/pages/${page}`, "page");
  get("/api/v1/me/notifications/count", "poll");
  sleep(2);
  get("/api/v1/search?q=plan", "search");
  get("/api/v1/feed", "feed");
  get("/api/v1/spaces", "directory");
  sleep(5);
}
