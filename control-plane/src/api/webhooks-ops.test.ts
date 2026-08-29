import assert from "node:assert/strict";
import test from "node:test";
import {
  INBOUND_ENDPOINT,
  asCreated,
  asPublic,
  canReplay,
  canTestOrReplay,
  hideCopiedSecret,
  lastDeliveryLabel,
  listTargetName,
  publicHasSecrets,
  webhookEndpoint,
  webhookStatus,
} from "./webhooks-ops.ts";

test("asPublic drops empty ids and fills list columns", () => {
  const rows = asPublic([
    {
      id: "w1",
      token_prefix: "wh_abc",
      endpoint: "https://hooks.example.invalid/goso",
      status: "active",
      last_delivery: { id: "d1", status: "done", at: "2026-08-30T00:00:00Z" },
    },
    { id: "", token_prefix: "x" },
  ]);
  assert.equal(rows.length, 1);
  assert.equal(rows[0].status, "active");
  assert.equal(rows[0].endpoint, "https://hooks.example.invalid/goso");
  assert.equal(lastDeliveryLabel(rows[0]), "done · 2026-08-30T00:00:00Z");
  assert.equal(publicHasSecrets(rows[0] as unknown as Record<string, unknown>), false);
});

test("webhookStatus maps revoked and failing last delivery", () => {
  assert.equal(webhookStatus({ status: "active", revoked: true }), "revoked");
  assert.equal(webhookStatus({ status: "active", last_delivery: { status: "dead" } }), "failing");
  assert.equal(webhookStatus({ status: "active", last_delivery: { status: "done" } }), "active");
  assert.equal(webhookStatus({}), "active");
});

test("webhookEndpoint falls back to inbound HTTP path", () => {
  assert.equal(webhookEndpoint({}), INBOUND_ENDPOINT);
  assert.equal(webhookEndpoint({ endpoint: " https://hooks.example.invalid/x " }), "https://hooks.example.invalid/x");
});

test("lastDeliveryLabel empty when never delivered", () => {
  assert.equal(lastDeliveryLabel({}), "");
  assert.equal(lastDeliveryLabel({ last_delivery: null }), "");
});

test("listTargetName prefers name then id", () => {
  assert.equal(listTargetName({ name: "ops", id: "w1" }), "ops");
  assert.equal(listTargetName({ id: "w1" }), "w1");
  assert.equal(listTargetName({}), "webhook");
});

test("asCreated keeps one-time secrets; hideCopiedSecret redacts after copy", () => {
  const created = asCreated({ id: "w1", token: "wh_secret", token_prefix: "wh_sec", hmac_key: "aabbcc" });
  assert.equal(created.token, "wh_secret");
  const hidden = hideCopiedSecret(created, "token");
  assert.equal(hidden.token, undefined);
  assert.equal(hidden.hmac_key, "aabbcc");
});

test("canTestOrReplay requires outbound http endpoint", () => {
  assert.equal(canTestOrReplay({ endpoint: INBOUND_ENDPOINT, status: "active" }), false);
  assert.equal(canTestOrReplay({ endpoint: "https://hooks.example.invalid/goso", status: "active" }), true);
  assert.equal(canTestOrReplay({ endpoint: "https://hooks.example.invalid/goso", revoked: true }), false);
});

test("canReplay needs a prior delivery", () => {
  const ep = "https://hooks.example.invalid/goso";
  assert.equal(canReplay({ endpoint: ep, status: "active" }), false);
  assert.equal(canReplay({ endpoint: ep, status: "active", last_delivery: { status: "done" } }), true);
});

test("publicHasSecrets catches GET leak shape", () => {
  assert.equal(publicHasSecrets({ id: "w1", token_prefix: "wh_ab" }), false);
  assert.equal(publicHasSecrets({ id: "w1", token: "wh_full" }), true);
  assert.equal(publicHasSecrets({ id: "w1", hmac_key: "deadbeef" }), true);
});
