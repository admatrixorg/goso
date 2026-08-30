import assert from "node:assert/strict";
import test from "node:test";
import { classifyPageState, inventoryBlocksMutation, listMetaCount } from "./page-state.ts";
import {
  asPublic,
  asPublicContext,
  filterTenants,
  formatWhen,
  memberConfirmMatch,
  publicHasSecrets,
  tenantConfirmMatch,
  tenantLabel,
} from "./tenants-ops.ts";
import type { Tenant } from "./tenants-ops.ts";

function row(over: Partial<Tenant> = {}): Tenant {
  return {
    id: "acme",
    name: "Acme",
    status: "active",
    master: false,
    created_at: "2026-08-30T12:00:00Z",
    members: [{ id: "tm_1", subject: "ops@acme.test", role: "admin" }],
    ...over,
  };
}

test("asPublicContext drops secret-shaped context", () => {
  assert.deepEqual(asPublicContext({ id: "default", name: "Master", status: "active", master: true }), {
    id: "default",
    name: "Master",
    status: "active",
    master: true,
  });
  assert.equal(asPublicContext({ id: "x", name: "sk-live-abcdefgh", master: false }), undefined);
  assert.equal(asPublicContext(undefined), undefined);
});

test("asPublic drops secret-shaped rows and keeps metadata", () => {
  const rows = asPublic([
    row(),
    row({ id: "beta", token: "wh_full" } as never),
    row({ id: "gamma", name: "sk-live-abcdefgh" }),
  ]);
  assert.equal(rows.length, 1);
  assert.equal(rows[0].id, "acme");
  assert.equal(rows[0].members?.[0].subject, "ops@acme.test");
  assert.equal(publicHasSecrets(rows[0]), false);
});

test("publicHasSecrets flags token payloads, not listing metadata", () => {
  assert.equal(publicHasSecrets(row()), false);
  assert.equal(publicHasSecrets({ id: "acme", token: "abc" }), true);
  assert.equal(publicHasSecrets({ id: "acme", api_key: "k" }), true);
  assert.equal(publicHasSecrets({ id: "acme", name: "Bearer abcdefghijk" }), true);
  assert.equal(publicHasSecrets({ id: "acme", members: [{ id: "tm_1", subject: "x", token: "t" }] }), true);
});

test("filterTenants matches slug name status", () => {
  const rows = [row(), row({ id: "beta", name: "Beta", status: "deactivated" })];
  assert.equal(filterTenants(rows, "").length, 2);
  assert.equal(filterTenants(rows, "acme")[0].id, "acme");
  assert.equal(filterTenants(rows, "DEACTIVATED").length, 1);
  assert.equal(filterTenants(rows, "nope").length, 0);
});

test("tenantConfirmMatch and memberConfirmMatch", () => {
  assert.equal(tenantConfirmMatch("acme", { id: "acme", name: "Acme" }), true);
  assert.equal(tenantConfirmMatch("Acme", { id: "acme", name: "Acme" }), true);
  assert.equal(tenantConfirmMatch("nope", { id: "acme", name: "Acme" }), false);
  assert.equal(tenantConfirmMatch("", { id: "acme", name: "Acme" }), false);
  assert.equal(memberConfirmMatch("ops@acme.test", { id: "tm_1", subject: "ops@acme.test" }), true);
  assert.equal(memberConfirmMatch("tm_1", { id: "tm_1", subject: "ops@acme.test" }), true);
  assert.equal(memberConfirmMatch("x", { id: "tm_1", subject: "ops@acme.test" }), false);
});

test("permission never claims tenant empty or enables create", () => {
  const s = classifyPageState({
    loading: false,
    loaded: false,
    error: new Error("401 unauthorized"),
    itemCount: 0,
  });
  assert.equal(s.kind, "permission");
  assert.equal(s.showEmpty, false);
  assert.equal(listMetaCount(s.kind, 0), null);
  assert.equal(inventoryBlocksMutation(s.kind), true);
});

test("true empty vs filtered empty after successful tenant list", () => {
  const empty = classifyPageState({ loading: false, loaded: true, error: null, itemCount: 0 });
  assert.equal(empty.kind, "empty");
  assert.equal(empty.showEmpty, true);
  assert.equal(inventoryBlocksMutation(empty.kind), false);
  const ready = classifyPageState({ loading: false, loaded: true, error: null, itemCount: 2 });
  assert.equal(filterTenants([row(), row({ id: "beta", name: "Beta" })], "nope").length, 0);
  assert.equal(ready.showItems, true);
});

test("stale keeps last-known tenants; permission still wins", () => {
  const stale = classifyPageState({
    loading: false,
    loaded: true,
    error: new Error("502 bad gateway"),
    itemCount: 1,
    keepStale: true,
  });
  assert.equal(stale.kind, "stale");
  assert.equal(stale.showItems, true);
  const perm = classifyPageState({
    loading: false,
    loaded: true,
    error: new Error("403 forbidden"),
    itemCount: 1,
    keepStale: true,
  });
  assert.equal(perm.kind, "permission");
  assert.equal(perm.showItems, false);
  assert.equal(inventoryBlocksMutation(perm.kind), true);
});

test("tenantLabel and formatWhen", () => {
  assert.equal(tenantLabel({ id: "acme", name: "Acme" }), "Acme");
  assert.equal(tenantLabel({ id: "acme", name: "  " }), "acme");
  assert.equal(formatWhen("", "n/a"), "n/a");
  assert.equal(formatWhen("not-a-date", "n/a"), "not-a-date");
  const shown = formatWhen("2026-08-30T12:00:00Z", "n/a");
  assert.notEqual(shown, "n/a");
  assert.ok(shown.includes("2026") || shown.includes("30"));
});
