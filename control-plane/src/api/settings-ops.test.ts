import assert from "node:assert/strict";
import test from "node:test";
import {
  editableValues,
  formFromSnapshot,
  publicHasSecrets,
  settingsConflictKind,
  validateGatewayForm,
  type GatewayConfig,
} from "./settings-ops.ts";

const snap: GatewayConfig = {
  updated_at: "2026-08-30T00:00:00Z",
  server: {
    log_level: { key: "log_level", value: "info", set: true, env_owned: false, editable: true },
    port: { key: "port", value: 8080, set: true, env_owned: true, editable: false },
  },
  auth: { token_set: { key: "token_set", value: true, set: true, env_owned: true, editable: false } },
  quota: { day_limit: { key: "day_limit", value: "0", set: true, env_owned: false, editable: true } },
  tools: { injection: { key: "injection", value: "log", set: true, env_owned: false, editable: true } },
  behavior: {
    heartbeat: { key: "heartbeat", value: "off", set: true, env_owned: false, editable: true },
    kg_extract: { key: "kg_extract", value: "off", set: true, env_owned: false, editable: true },
    cache_mode: { key: "cache_mode", value: "", set: false, env_owned: false, editable: true },
  },
};

test("formFromSnapshot and editableValues skip env-owned", () => {
  const form = formFromSnapshot(snap);
  assert.equal(form.log_level, "info");
  assert.equal(form.quota_day, "0");
  const envLocked: GatewayConfig = {
    ...snap,
    quota: { day_limit: { key: "day_limit", value: "9", set: true, env_owned: true, editable: false } },
  };
  const values = editableValues({ ...form, quota_day: "4", log_level: "debug" }, envLocked);
  assert.equal(values.log_level, "debug");
  assert.equal(values.quota_day, undefined);
});

test("validateGatewayForm", () => {
  const form = formFromSnapshot(snap);
  assert.equal(validateGatewayForm(form, { quota_day: "3" }), null);
  assert.equal(validateGatewayForm({ ...form, quota_day: "-1" }, { quota_day: "-1" }), "settings.invalidQuota");
  assert.equal(validateGatewayForm({ ...form, log_level: "trace" }, { log_level: "trace" }), "settings.invalidLogLevel");
});

test("settingsConflictKind", () => {
  assert.equal(settingsConflictKind(new Error('409 {"error":"config was modified"}')), "conflict");
  assert.equal(settingsConflictKind(new Error('409 {"error":"field is env-owned: quota_day"}')), "env_owned");
  assert.equal(settingsConflictKind(new Error("400 bad")), null);
});

test("publicHasSecrets flags token-shaped values, not token_set", () => {
  assert.equal(publicHasSecrets(snap), false);
  assert.equal(
    publicHasSecrets({
      auth: { token: { key: "token", value: "goso-admin-abcdef", set: true } },
    }),
    true,
  );
  assert.equal(
    publicHasSecrets({
      auth: { token_set: { key: "token_set", value: true, set: true } },
    }),
    false,
  );
});
