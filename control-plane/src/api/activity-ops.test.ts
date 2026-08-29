import assert from "node:assert/strict";
import test from "node:test";
import {
  activityQs,
  asPublicRecord,
  localToRfc3339,
  parseDetail,
  publicHasSecrets,
  publicMeta,
  uniqueField,
  type ActivityRecord,
} from "./activity-ops.ts";

function row(over: Partial<ActivityRecord> = {}): ActivityRecord {
  return {
    seq: 1,
    id: "ar-1",
    action: "update",
    actor: "operator",
    entity: "agent",
    entity_id: "ag1",
    ip: "10.0.0.1",
    ts: "2026-08-30T12:00:00Z",
    after: { enabled: true },
    ...over,
  };
}

test("asPublicRecord drops secret keys and token shapes", () => {
  const pub = asPublicRecord(
    row({
      after: {
        enabled: true,
        api_key: "sk-live-abcdefghijk",
        token: "super-secret",
        body: "secret-chat",
        note: "Bearer abcdefghijk",
      },
    }),
  );
  assert.ok(pub);
  assert.equal(pub.after?.enabled, true);
  assert.equal(pub.after?.api_key, undefined);
  assert.equal(pub.after?.token, undefined);
  assert.equal(pub.after?.body, undefined);
  assert.equal(pub.after?.note, "[redacted]");
  assert.equal(publicHasSecrets(pub.after), false);
});

test("publicHasSecrets flags credential values, not audit metadata", () => {
  assert.equal(publicHasSecrets({ action: "update", entity: "agent", after: { enabled: true } }), false);
  assert.equal(publicHasSecrets({ after: { token: "abc" } }), true);
  assert.equal(publicHasSecrets({ after: { note: "sk-live-abcdefghijk" } }), true);
  assert.equal(publicMeta({ status: "paired", password: "x" })?.status, "paired");
  assert.equal(publicMeta({ password: "x" }), undefined);
});

test("parseDetail is schema-safe and skips payload keys", () => {
  const details = parseDetail(
    row({
      after: { ok: true, enabled: false, arguments: { x: 1 }, token: "nope" },
    }),
  );
  const keys = details.map((d) => d.key);
  assert.ok(keys.includes("action"));
  assert.ok(keys.includes("after.ok"));
  assert.ok(keys.includes("after.enabled"));
  assert.equal(keys.includes("after.arguments"), false);
  assert.equal(keys.includes("after.token"), false);
});

test("activityQs and uniqueField", () => {
  assert.equal(activityQs({ action: "create", limit: 25 }), "?action=create&limit=25");
  assert.equal(activityQs({ before: 12 }), "?before=12");
  const actors = uniqueField([row(), row({ actor: "alice", seq: 2 })], "actor");
  assert.deepEqual(actors, ["alice", "operator"]);
  const iso = localToRfc3339("2026-08-30T12:00");
  assert.ok(iso.includes("2026-08-30"));
});
