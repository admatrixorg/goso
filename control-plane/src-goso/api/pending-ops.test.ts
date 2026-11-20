import assert from "node:assert/strict";
import test from "node:test";
import {
  agentLabel,
  asPublic,
  compactBlocked,
  formatAge,
  groupLabel,
  pendingConfirmMatch,
  pendingWriteKind,
  previewLine,
  publicHasSecrets,
} from "./pending-ops.ts";

test("asPublic drops secret-shaped rows and keeps counts", () => {
  const rows = asPublic([
    { id: "pg_1", channel: "telegram", dest: "-1001", agent: "Support", count: 3, age_ms: 4000 },
    { id: "pg_2", channel: "discord", dest: "c1", count: 1, token: "wh_full" } as never,
  ]);
  assert.equal(rows.length, 1);
  assert.equal(rows[0].id, "pg_1");
  assert.equal(rows[0].count, 3);
  assert.equal(publicHasSecrets(rows[0]), false);
});

test("publicHasSecrets flags token/code payloads, not listing metadata", () => {
  assert.equal(publicHasSecrets({ id: "pg_1", channel: "telegram", dest: "1", count: 2 }), false);
  assert.equal(publicHasSecrets({ id: "pg_1", token: "abc" }), true);
  assert.equal(publicHasSecrets({ id: "pg_1", code: "1234" }), true);
  assert.equal(publicHasSecrets({ id: "pg_1", content: "hello" }), true);
  assert.equal(publicHasSecrets({ id: "pg_1", dest: "sk-live-abcdefgh" }), true);
  assert.equal(publicHasSecrets({ id: "pg_1", compacted: true, compacting: false }), false);
});

test("pendingConfirmMatch accepts id, dest, or channel:dest", () => {
  const g = { id: "pg_9", channel: "slack", dest: "d1" };
  assert.equal(pendingConfirmMatch("pg_9", g), true);
  assert.equal(pendingConfirmMatch("d1", g), true);
  assert.equal(pendingConfirmMatch("slack:d1", g), true);
  assert.equal(pendingConfirmMatch(" slack:d1 ", g), true);
  assert.equal(pendingConfirmMatch("", g), false);
  assert.equal(pendingConfirmMatch("other", g), false);
});

test("labels, age, and preview", () => {
  assert.equal(groupLabel({ channel: "telegram", dest: "-1" }), "telegram:-1");
  assert.equal(agentLabel({ agent: "Desk" }, "n/a"), "Desk");
  assert.equal(agentLabel({ agent_id: "ag-1" }, "n/a"), "ag-1");
  assert.equal(agentLabel({}, "n/a"), "n/a");
  assert.equal(formatAge(1500), "1s");
  assert.equal(formatAge(120_000), "2m");
  assert.equal(formatAge(3_600_000), "1h");
  assert.equal(formatAge(90_000_000), "1d");
  const line = previewLine({ id: "pg_1", channel: "telegram", dest: "9", count: 4, age_ms: 5000 }, "n/a");
  assert.match(line, /telegram:9/);
  assert.match(line, /n\/a/);
  assert.match(line, /4/);
});

test("pendingWriteKind classifies compact/clear API failures", () => {
  assert.equal(pendingWriteKind(new Error("409 {\"error\":\"compact in progress\"}")), "busy");
  assert.equal(pendingWriteKind(new Error("401 {\"error\":\"unauthorized\"}")), "permission");
  assert.equal(pendingWriteKind(new Error("403 lite: channels off")), "permission");
  assert.equal(pendingWriteKind(new Error("400 confirm does not match")), "mismatch");
  assert.equal(pendingWriteKind(new Error("404 not found")), "missing");
  assert.equal(pendingWriteKind(new Error("500 boom")), "error");
  assert.equal(compactBlocked({ compacting: true }, false), true);
  assert.equal(compactBlocked({ compacting: false }, true), true);
  assert.equal(compactBlocked({ compacting: false }, false), false);
});
