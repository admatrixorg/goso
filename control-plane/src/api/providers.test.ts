import assert from "node:assert/strict";
import test from "node:test";
import {
  canClearProviderKey,
  filterProviders,
  formatProviderTest,
  isEnvOwned,
  isProviderEnabled,
  providerWriteBody,
  uniqueProviderTypes,
  type ProviderInfo,
} from "./provider-ops.ts";

const rows: ProviderInfo[] = [
  { name: "echo", type: "echo", base_url: "", model: "echo", key_set: false, source: "env", enabled: true },
  { name: "acme", type: "openai-compat", base_url: "http://127.0.0.1:9", model: "gpt-unit", key_set: true, source: "sqlite", enabled: true },
  { name: "off", type: "anthropic", base_url: "", model: "claude", key_set: false, source: "sqlite", enabled: false },
];

test("filterProviders matches name, type, model, url", () => {
  assert.equal(filterProviders(rows, { query: "acme" }).map((p) => p.name).join(), "acme");
  assert.equal(filterProviders(rows, { query: "CLAUDE" }).map((p) => p.name).join(), "off");
  assert.equal(filterProviders(rows, { query: "127.0.0.1" }).map((p) => p.name).join(), "acme");
  assert.equal(filterProviders(rows, { type: "echo" }).map((p) => p.name).join(), "echo");
  assert.equal(filterProviders(rows, { source: "sqlite" }).length, 2);
  assert.equal(filterProviders(rows, { query: "  ", type: "", source: "" }).length, 3);
});

test("filterProviders enabled uses optional flag", () => {
  assert.equal(filterProviders(rows, { enabled: "on" }).map((p) => p.name).join(), "echo,acme");
  assert.equal(filterProviders(rows, { enabled: "off" }).map((p) => p.name).join(), "off");
  assert.equal(isProviderEnabled({ enabled: undefined }), true);
  assert.equal(isProviderEnabled({ enabled: false }), false);
});

test("canClearProviderKey is sqlite plus key_set", () => {
  assert.equal(canClearProviderKey({ source: "sqlite", key_set: true }), true);
  assert.equal(canClearProviderKey({ source: "sqlite", key_set: false }), false);
  assert.equal(canClearProviderKey({ source: "env", key_set: true }), false);
  assert.equal(isEnvOwned({ source: "env" }), true);
  assert.equal(isEnvOwned({ source: "sqlite" }), false);
});

test("providerWriteBody omits blank api_key", () => {
  assert.deepEqual(providerWriteBody({ name: " acme ", type: "echo", base_url: "u", model: "m", api_key: "  ", enabled: true }), {
    name: "acme",
    type: "echo",
    base_url: "u",
    model: "m",
    enabled: true,
  });
  assert.deepEqual(providerWriteBody({ type: "echo", api_key: "  secret  " }).api_key, "secret");
});

test("formatProviderTest redacts errors and never echoes keys", () => {
  const view = formatProviderTest({
    ok: false,
    latency_ms: 12,
    error: '401 Bearer unit-key {"api_key":"unit-key"}',
    models: ["a", "b"],
    reply: "sk-live-ABCDEFG hi",
  });
  assert.equal(view.ok, false);
  assert.equal(view.latency_ms, 12);
  assert.equal(view.models.join(), "a,b");
  assert.equal(view.error.includes("unit-key"), false);
  assert.equal(view.reply.includes("sk-live-ABCDEFG"), false);
  assert.match(view.error, /Bearer \[redacted\]/);
  const extra = formatProviderTest({ ok: true, latency_ms: 1, api_key: "secret-value" } as never);
  assert.equal(JSON.stringify(extra).includes("secret-value"), false);
  assert.deepEqual(Object.keys(extra).sort(), ["error", "latency_ms", "models", "ok", "reply"]);
});

test("uniqueProviderTypes keeps catalog then extras", () => {
  const types = uniqueProviderTypes([{ type: "openai-compat" }, { type: "custom-x" }, { type: "" }]);
  assert.equal(types.includes("openai-compat"), true);
  assert.equal(types.includes("custom-x"), true);
  assert.equal(types[0], "openai-compat");
});
