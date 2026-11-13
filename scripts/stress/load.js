// SPDX-License-Identifier: Apache-2.0
// k6 stress test of one instance seeded by seed.py. k6 is a test tool, not a dependency.
//   k6 run -e BASE_URL=http://app:8080 -e SEED=seeded.json -e SCENARIO=mix load.js
// The seeded people's password is read from the file SEED + ".password".
// Scenarios:
//   mix     about 10% of 5,000 people active, the pattern of blueprint 04.8: reading,
//           about 40% of the time in search, some saving, comments and uploads
//   worst   all 5,000 logged in and active at once, with short pauses
//   search  people who mostly search
//   step    the mix, raising the number of active people until it breaks
//   samepage 50 people saving one page at the same time, as many open tabs would
import http from "k6/http";
import { check, sleep } from "k6";
import { Counter } from "k6/metrics";
import exec from "k6/execution";
import { SharedArray } from "k6/data";

const base = __ENV.BASE_URL;
// Each list is held once for all simulated people: a copy per person would need
// gigabytes at 5,000 people.
const list = (name) => new SharedArray(name, () => JSON.parse(open(__ENV.SEED))[name]);
const seed = {
  readable_pages: list("readable_pages"),
  writable_pages: list("writable_pages"),
  readable_spaces: list("readable_spaces"),
  common_words: list("common_words"),
  rare_words: list("rare_words"),
  mid_words: list("mid_words"),
  people: new SharedArray("people", () => [JSON.parse(open(__ENV.SEED)).people])[0],
};
const password = open(__ENV.SEED + ".password").trim();
const scenario = __ENV.SCENARIO || "mix";
const busy = new Counter("busy_503");
const failedLogins = new Counter("failed_logins");

const minutes = (n) => `${n}m`;
const plans = {
  mix: { executor: "ramping-vus", stages: [
    { duration: minutes(3), target: 500 }, { duration: minutes(10), target: 500 }, { duration: "1m", target: 0 }] },
  // RAMP and HOLD shorten a run when time is short, as in the second test.
  worst: { executor: "ramping-vus", stages: [
    { duration: __ENV.RAMP || minutes(6), target: 5000 }, { duration: __ENV.HOLD || minutes(6), target: 5000 },
    { duration: "1m", target: 0 }] },
  search: { executor: "ramping-vus", stages: [
    { duration: minutes(1), target: 200 }, { duration: minutes(5), target: 200 }, { duration: "30s", target: 0 }] },
  // On a server like the one in the report, 500 active people already fill PostgreSQL,
  // so the steps start low.
  step: { executor: "ramping-vus", stages: [
    { duration: minutes(1), target: 50 }, { duration: minutes(3), target: 50 },
    { duration: minutes(1), target: 100 }, { duration: minutes(3), target: 100 },
    { duration: minutes(1), target: 200 }, { duration: minutes(3), target: 200 },
    { duration: minutes(1), target: 350 }, { duration: minutes(3), target: 350 },
    { duration: minutes(1), target: 500 }, { duration: minutes(3), target: 500 },
    { duration: "30s", target: 0 }] },
  samepage: { executor: "constant-vus", vus: 50, duration: minutes(3) },
  smoke: { executor: "constant-vus", vus: 20, duration: minutes(1) },
  // One level of the step test on its own, so each level has its own summary.
  level: { executor: "constant-vus", vus: Number(__ENV.VUS || 100), duration: __ENV.HOLD || minutes(4) },
};

// The whole summary goes to a file, for summarise.py.
export function handleSummary(data) {
  return { [`summary-${scenario}${__ENV.VUS || ""}.json`]: JSON.stringify(data) };
}

export const options = {
  scenarios: { [scenario]: plans[scenario].executor === "ramping-vus"
    ? Object.assign({ gracefulRampDown: "30s" }, plans[scenario]) : plans[scenario] },
  // Bodies are kept only where the script reads them (responseType below).
  discardResponseBodies: true,
  // A person logs in once and stays logged in, as in a browser.
  noCookiesReset: true,
  summaryTrendStats: ["avg", "p(50)", "p(95)", "p(99)", "max"],
  thresholds: {
    "http_req_duration{route:page}": ["p(95)<300"],
    "http_req_duration{route:quick}": ["p(95)<500"],
    "http_req_duration{route:search}": ["p(95)<500"],
    "http_req_duration{route:publish}": ["p(95)<1000"],
    // The rest are listed so k6 keeps a number per route; their limits are loose.
    "http_req_duration{route:attachments}": ["p(95)<1000"],
    "http_req_duration{route:poll}": ["p(95)<1000"],
    "http_req_duration{route:tree}": ["p(95)<1000"],
    "http_req_duration{route:feed}": ["p(95)<1000"],
    "http_req_duration{route:draft}": ["p(95)<1000"],
    "http_req_duration{route:comment}": ["p(95)<1000"],
    "http_req_duration{route:upload}": ["p(95)<2000"],
    "http_req_duration{route:login}": ["p(95)<2000"],
  },
};

// Pauses: a person reads, thinks and types. The worst case shortens them.
const pause = scenario === "worst" ? 0.4 : 1;
const searchShare = scenario === "search" ? 0.9 : 0.4;

const json = { headers: { "Content-Type": "application/json", Origin: base } };
const pick = (list) => list[Math.floor(Math.random() * list.length)];

function get(path, route, keepBody) {
  // The name tag groups every address of a route, so k6 keeps one series per route.
  const r = http.get(`${base}${path}`, { tags: { route, name: route }, responseType: keepBody ? "text" : "none" });
  if (r.status === 503) busy.add(1, { route });
  check(r, { [`${route} answered`]: (x) => x.status === 200 || x.status === 503 });
  return r;
}

function login() {
  const n = (exec.vu.idInTest - 1) % seed.people;
  const email = `person${String(n).padStart(5, "0")}@example.test`;
  const r = http.post(`${base}/api/v1/auth/login`, JSON.stringify({ email, password }), Object.assign({ tags: { route: "login", name: "login" } }, json));
  if (r.status !== 200) failedLogins.add(1);
  check(r, { "logged in": (x) => x.status === 200 });
}

function searching() {
  const word = Math.random() < 0.3 ? pick(seed.common_words) : Math.random() < 0.5 ? pick(seed.mid_words) : pick(seed.rare_words);
  // Typing: the quick box asks after the third letter, as the frontend does after a pause.
  for (let n = 3; n <= word.length; n += 2) {
    get(`/api/v1/search/quick?q=${encodeURIComponent(word.slice(0, n))}`, "quick");
    sleep(0.3 * pause);
  }
  if (Math.random() < 0.3) {
    get(`/api/v1/search?q=${encodeURIComponent(word)}`, "search");
    sleep(3 * pause);
  }
}

function reading() {
  const page = pick(seed.readable_pages);
  const wantsTree = Math.random() < 0.2;
  const r = get(`/api/v1/pages/${page}`, "page", wantsTree);
  get(`/api/v1/pages/${page}/attachments`, "attachments");
  get("/api/v1/me/notifications/count", "poll");
  if (wantsTree) {
    const space = r.status === 200 ? r.json("space_id") : pick(seed.readable_spaces);
    get(`/api/v1/spaces/${space}/tree`, "tree");
  }
  if (Math.random() < 0.15) get("/api/v1/feed", "feed");
  sleep(8 * pause);
}

function writing(only) {
  const page = only || pick(seed.writable_pages);
  const r = get(`/api/v1/pages/${page}`, "page", true);
  if (r.status !== 200) return;
  const doc = r.json("document");
  const title = r.json("title");
  const version = r.json("current_version");
  doc.content.push({ type: "paragraph", content: [{ type: "text", text: `Added under load by ${exec.vu.idInTest} at ${Date.now()}.` }] });
  const draft = http.put(`${base}/api/v1/pages/${page}/draft`, JSON.stringify({ title, document: doc, base_version: version }), Object.assign({ tags: { route: "draft", name: "draft" } }, json));
  if (draft.status === 503) busy.add(1, { route: "draft" });
  sleep(2 * pause);
  const pub = http.post(`${base}/api/v1/pages/${page}/publish`, JSON.stringify({ title, document: doc, base_version: version }), Object.assign({ tags: { route: "publish", name: "publish" } }, json));
  if (pub.status === 503) busy.add(1, { route: "publish" });
  // 409 is someone else saving first: a real answer, not a failure.
  check(pub, { "published or conflict": (x) => x.status === 200 || x.status === 409 || x.status === 503 });
}

function commenting() {
  const page = pick(seed.readable_pages);
  const r = http.post(`${base}/api/v1/pages/${page}/comments`, JSON.stringify({ body: { type: "doc", content: [
    { type: "paragraph", content: [{ type: "text", text: "A comment made under load." }] }] } }), Object.assign({ tags: { route: "comment", name: "comment" } }, json));
  if (r.status === 503) busy.add(1, { route: "comment" });
  check(r, { "commented": (x) => x.status === 201 || x.status === 200 || x.status === 503 });
}

function uploading() {
  const page = pick(seed.writable_pages);
  const body = "x".repeat(50000 + Math.floor(Math.random() * 200000));
  const r = http.post(`${base}/api/v1/pages/${page}/attachments?filename=load-${exec.vu.idInTest}-${Date.now()}.txt`, body,
    { headers: { "Content-Type": "application/octet-stream", Origin: base }, tags: { route: "upload", name: "upload" } });
  if (r.status === 503) busy.add(1, { route: "upload" });
  check(r, { "uploaded": (x) => x.status === 201 || x.status === 200 || x.status === 503 });
}

export default function () {
  if (exec.vu.iterationInScenario === 0) {
    login();
    sleep(Math.random() * 3);
  }
  if (scenario === "samepage") {
    writing(seed.writable_pages[0]);
    return;
  }
  const roll = Math.random();
  if (roll < searchShare) searching();
  else if (roll < searchShare + 0.03) writing();
  else if (roll < searchShare + 0.05) commenting();
  else if (roll < searchShare + 0.055) uploading();
  else reading();
}
