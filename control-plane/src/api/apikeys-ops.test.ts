import assert from "node:assert/strict";
import test from "node:test";
import { classifyPageState, inventoryBlocksMutation, listMetaCount } from "./page-state.ts";
import {
  asCreated,
  asPublic,
  filterKeys,
  formatWhen,
  hideCopiedSecret,
  isKnownScope,
  keyConfirmMatch,
  keyLabel,
  maskedPrefix,
  publicHasSecrets,
  toggleScope,
  usageLabel,
} from "./apikeys-ops.ts";
import type { ApiKey } from "./apikeys-ops.ts";

function row(over: Partial<ApiKey> = {}): ApiKey {
  return {
    id: "ak_1",
    name: "ops",
    prefix: "gk_abcd1234",
    tenant_id: "default",
    scopes: ["read", "write"],
    status: "active",
    use_count: 2,
    created_at: "2026-08-30T12:00:00Z",
    last_used_at: "2026-08-30T13:00:00Z",
    ...over,
  };
}

test("asPublic drops secret-shaped rows and keeps prefix metadata", () => {
  const rows = asPublic([
    row(),
    row({ id: "ak_2", secret: "gk_fullsecretvalue" } as never),
    row({ id: "ak_3", name: "sk-live-abcdefgh" }),
  ]);
  assert.equal(rows.length, 1);
  assert.equal(rows[0].id, "ak_1");
  assert.equal(rows[0].prefix, "gk_abcd1234");
  assert.deepEqual(rows[0].scopes, ["read", "write"]);
  assert.equal(publicHasSecrets(rows[0]), false);
});

test("publicHasSecrets flags token payloads, not listing metadata", () => {
  assert.equal(publicHasSecrets(row()), false);
  assert.equal(publicHasSecrets({ id: "ak_1", secret: "gk_full" }), true);
  assert.equal(publicHasSecrets({ id: "ak_1", api_key: "k" }), true);
  assert.equal(publicHasSecrets({ id: "ak_1", hash: "abc" }), true);
  assert.equal(publicHasSecrets({ id: "ak_1", name: "Bearer abcdefghijk" }), true);
  assert.equal(publicHasSecrets({ id: "ak_1", name: "gk_" + "ab".repeat(12) }), true);
  assert.equal(publicHasSecrets({ id: "ak_1", prefix: "gk_abcd1234" }), false);
});

test("asCreated keeps secret once; hideCopiedSecret clears it after copy hide or navigation", () => {
  const last = asCreated({ ...row(), secret: "gk_abcd1234deadbeef" });
  assert.equal(last?.secret, "gk_abcd1234deadbeef");
  assert.equal(asCreated(row()), null);
  const hidden = hideCopiedSecret(last!);
  assert.equal(hidden.secret, undefined);
  assert.equal(hidden.prefix, "gk_abcd1234");
  assert.equal(JSON.stringify(hidden).includes("gk_abcd1234deadbeef"), false);
});

test("permission blocks create/revoke and does not claim zero keys", () => {
  const s = classifyPageState({
    loading: false,
    loaded: false,
    error: new Error("401 unauthorized"),
    itemCount: 0,
  });
  assert.equal(s.kind, "permission");
  assert.equal(s.showEmpty, false);
  assert.equal(inventoryBlocksMutation(s.kind), true);
  assert.equal(listMetaCount(s.kind, 0), null);
});

test("filterKeys matches name prefix status scope tenant", () => {
  const rows = [row(), row({ id: "ak_2", name: "ci", status: "revoked", scopes: ["admin"], tenant_id: "acme" })];
  assert.equal(filterKeys(rows, "").length, 2);
  assert.equal(filterKeys(rows, "ops")[0].id, "ak_1");
  assert.equal(filterKeys(rows, "REVOKED").length, 1);
  assert.equal(filterKeys(rows, "admin").length, 1);
  assert.equal(filterKeys(rows, "acme").length, 1);
  assert.equal(filterKeys(rows, "nope").length, 0);
});

test("keyConfirmMatch and labels", () => {
  assert.equal(keyConfirmMatch("ak_1", { id: "ak_1", name: "ops", prefix: "gk_abcd1234" }), true);
  assert.equal(keyConfirmMatch("ops", { id: "ak_1", name: "ops", prefix: "gk_abcd1234" }), true);
  assert.equal(keyConfirmMatch("gk_abcd1234", { id: "ak_1", name: "ops", prefix: "gk_abcd1234" }), true);
  assert.equal(keyConfirmMatch("nope", { id: "ak_1", name: "ops", prefix: "gk_abcd1234" }), false);
  assert.equal(keyConfirmMatch("", { id: "ak_1", name: "ops", prefix: "gk_abcd1234" }), false);
  assert.equal(keyLabel({ id: "ak_1", name: "ops", prefix: "gk_x" }), "ops");
  assert.equal(keyLabel({ id: "ak_1", name: "  ", prefix: "gk_x" }), "gk_x");
  assert.equal(maskedPrefix("gk_abcd1234"), "gk_abcd1234…");
  assert.equal(isKnownScope("read"), true);
  assert.equal(isKnownScope("root"), false);
  assert.deepEqual(toggleScope(["read"], "write"), ["read", "write"]);
  assert.deepEqual(toggleScope(["read", "write"], "read"), ["write"]);
});

test("usageLabel and formatWhen", () => {
  assert.equal(usageLabel({ use_count: 0 }, "never"), "never");
  assert.equal(formatWhen("", "n/a"), "n/a");
  assert.equal(formatWhen("not-a-date", "n/a"), "not-a-date");
  const used = usageLabel({ use_count: 2, last_used_at: "2026-08-30T13:00:00Z" }, "never");
  assert.equal(used.startsWith("2 · "), true);
});
