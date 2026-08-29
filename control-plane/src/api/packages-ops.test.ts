import assert from "node:assert/strict";
import test from "node:test";
import {
  allowConfirmMatch,
  asPublicCreds,
  asPublicSnapshot,
  cliConfirmMatch,
  filterByEco,
  jobActive,
  latestJob,
  pinValid,
  pkgConfirmMatch,
  pkgLabel,
  publicHasSecrets,
  snapshotHasSecrets,
  type Pkg,
} from "./packages-ops.ts";

const pkg = (over: Partial<Pkg> = {}): Pkg => ({
  id: "pk_1",
  ecosystem: "python",
  name: "httpx",
  version: "0.27.2",
  status: "installed",
  ...over,
});

test("asPublicSnapshot drops secret-shaped rows and keeps metadata", () => {
  const snap = asPublicSnapshot({
    runtimes: [{ name: "python", ecosystem: "python", present: true, compatible: true, version: "3.12.1" }],
    allowlist: [{ id: "al_1", ecosystem: "python", name: "httpx", pin: "0.27.2" }],
    packages: [pkg(), pkg({ id: "pk_2", secret: "ghp_abc" } as never)],
    jobs: [{ id: "pj_1", action: "install", package_id: "pk_1", ecosystem: "python", name: "httpx", version: "0.27.2", status: "succeeded", progress: 100, log: ["done"] }],
    credentials: [{ kind: "github", set: true, token: "ghp_live" } as never, { kind: "npm", set: false }],
  });
  assert.equal(snap.packages.length, 1);
  assert.equal(snap.packages[0].id, "pk_1");
  assert.equal(snap.credentials.length, 3);
  assert.equal(snap.credentials[0].kind, "github");
  assert.equal(snap.credentials[0].set, false);
  assert.equal("token" in snap.credentials[0], false);
});

test("publicHasSecrets and snapshotHasSecrets", () => {
  assert.equal(publicHasSecrets(pkg()), false);
  assert.equal(publicHasSecrets({ id: "pk_1", token: "x" }), true);
  assert.equal(publicHasSecrets({ id: "pk_1", log: ["Bearer abcdefghijk"] }), true);
  assert.equal(snapshotHasSecrets({ packages: [pkg()] }), false);
  assert.equal(snapshotHasSecrets({ packages: [{ id: "pk_1", secret: "x" }] }), true);
});

test("pinValid rejects latest and ranges", () => {
  assert.equal(pinValid("0.27.2"), true);
  assert.equal(pinValid("v1.2.3"), true);
  assert.equal(pinValid("latest"), false);
  assert.equal(pinValid("*"), false);
  assert.equal(pinValid("^1.0.0"), false);
  assert.equal(pinValid(""), false);
});

test("filters confirms and labels", () => {
  const rows = [pkg(), pkg({ id: "pk_2", ecosystem: "node", name: "left-pad" })];
  assert.equal(filterByEco(rows, "python").length, 1);
  assert.equal(filterByEco(rows, "python", "HTTPX").length, 1);
  assert.equal(pkgConfirmMatch("pk_1", pkg()), true);
  assert.equal(pkgConfirmMatch("httpx", pkg()), true);
  assert.equal(pkgConfirmMatch("python/httpx", pkg()), true);
  assert.equal(pkgConfirmMatch("nope", pkg()), false);
  assert.equal(allowConfirmMatch("httpx", { id: "al_1", name: "httpx", ecosystem: "python" }), true);
  assert.equal(cliConfirmMatch("github", "github"), true);
  assert.equal(cliConfirmMatch("npm", "github"), false);
  assert.equal(pkgLabel(pkg()), "python/httpx@0.27.2");
});

test("jobActive and latestJob", () => {
  const jobs = [
    { id: "pj_1", action: "install", package_id: "pk_1", ecosystem: "python", name: "httpx", version: "0.27.2", status: "succeeded", progress: 100, log: [] },
    { id: "pj_2", action: "install", package_id: "pk_1", ecosystem: "python", name: "httpx", version: "0.27.2", status: "running", progress: 40, log: [] },
  ];
  assert.equal(jobActive(jobs[1]), true);
  assert.equal(jobActive(jobs[0]), false);
  assert.equal(latestJob(jobs, "pk_1")?.id, "pj_2");
});

test("asPublicCreds always returns three kinds without secrets", () => {
  const rows = asPublicCreds([{ kind: "github", set: true, updated_at: "2026-08-30T00:00:00Z" }]);
  assert.deepEqual(rows.map((r) => r.kind), ["github", "npm", "pypi"]);
  assert.equal(rows[0].set, true);
  assert.equal(rows[1].set, false);
});
