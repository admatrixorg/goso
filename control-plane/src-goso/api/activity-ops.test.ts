import assert from "node:assert/strict";
import test from "node:test";
import {
  activityActionsBlocked,
  activityCursorMeta,
  activityFilteredEmpty,
  activityFiltersActive,
  activityQs,
  asPublicRecord,
  classifyActivityList,
  localToRfc3339,
  parseDetail,
  publicHasSecrets,
  publicMeta,
  uniqueField,
  type ActivityPage,
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
  const flags = publicMeta({ secret_set: true, key_set: true, api_key: "sk-live-abcdefghijk" });
  assert.equal(flags?.secret_set, true);
  assert.equal(flags?.key_set, true);
  assert.equal(flags?.api_key, undefined);
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
});

test("permission vs empty vs filtered empty", () => {
  const perm = classifyActivityList({
    loading: false,
    loaded: false,
    error: new Error("403 forbidden"),
    itemCount: 0,
  });
  assert.equal(perm.kind, "permission");
  assert.equal(perm.showEmpty, false);
  assert.equal(activityActionsBlocked(perm.kind), true);
  assert.equal(activityFilteredEmpty(perm, true), false);
  const empty = classifyActivityList({ loading: false, loaded: true, error: null, itemCount: 0 });
  assert.equal(empty.kind, "empty");
  assert.equal(activityFilteredEmpty(empty, false), false);
  assert.equal(activityFilteredEmpty(empty, true), true);
  assert.equal(activityFiltersActive({ action: "update", range: "all" }), true);
  assert.equal(activityFiltersActive({ range: "all" }), false);
});

test("cursor pagination provenance", () => {
  const page: ActivityPage = { records: [row()], total: 37, limit: 25, before: 12, next_before: 40 };
  const meta = activityCursorMeta(page, 1);
  assert.equal(meta.hasPrev, true);
  assert.equal(meta.hasNext, true);
  assert.equal(meta.shown, 1);
  assert.equal(meta.total, 37);
  assert.equal(meta.before, 12);
  assert.equal(meta.nextBefore, 40);
  const first = activityCursorMeta({ records: [], total: 0, limit: 25 }, 0);
  assert.equal(first.hasPrev, false);
  assert.equal(first.hasNext, false);
});

test("stale keeps records; leak flag is not permission", () => {
  const stale = classifyActivityList({
    loading: false,
    loaded: true,
    error: new Error("502"),
    itemCount: 4,
  });
  assert.equal(stale.kind, "stale");
  assert.equal(stale.showItems, true);
  assert.equal(activityActionsBlocked(stale.kind), false);
  assert.equal(publicHasSecrets({ after: { enabled: true } }), false);
  const iso = localToRfc3339("2026-08-30T12:00");
  assert.ok(iso.includes("2026-08-30"));
});
