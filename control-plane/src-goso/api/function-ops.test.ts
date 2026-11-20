import assert from "node:assert/strict";
import test from "node:test";
import {
  configuredLabel,
  connectorWriteBody,
  formatConnectorTest,
  isConnectorEnabled,
  isConnectorEnvOwned,
  normalizeTransport,
  toolListLeaksSecret,
} from "./function-ops.ts";

test("normalizeTransport maps stdio/sse/http onto stored names", () => {
  assert.equal(normalizeTransport("stdio"), "mcp-stdio");
  assert.equal(normalizeTransport("SSE"), "mcp-http");
  assert.equal(normalizeTransport("http"), "http");
  assert.equal(normalizeTransport("streamable-http"), "mcp-http");
  assert.equal(normalizeTransport("nope"), "");
});

test("connectorWriteBody omits blank token", () => {
  assert.deepEqual(
    connectorWriteBody({ name: " crm ", transport: "sse", endpoint: " http://127.0.0.1:9 ", token: "  ", enabled: true }),
    { name: "crm", transport: "mcp-http", endpoint: "http://127.0.0.1:9", enabled: true },
  );
  assert.equal(connectorWriteBody({ token: "  secret  " }).token, "secret");
});

test("env-owned and enabled helpers", () => {
  assert.equal(isConnectorEnvOwned({ source: "env" }), true);
  assert.equal(isConnectorEnvOwned({ env_owned: true, source: "sqlite" }), true);
  assert.equal(isConnectorEnvOwned({ source: "sqlite" }), false);
  assert.equal(isConnectorEnabled({ enabled: false }), false);
  assert.equal(isConnectorEnabled({}), true);
  assert.equal(configuredLabel(true), "configured");
  assert.equal(configuredLabel(false), "not_configured");
});

test("formatConnectorTest redacts secrets and drops extra token fields", () => {
  const view = formatConnectorTest({
    ok: false,
    latency_ms: 9,
    health: "unavailable",
    error: '401 Bearer unit-key {"token":"unit-key"}',
    token: "unit-key",
  } as never);
  assert.equal(view.ok, false);
  assert.equal(view.latency_ms, 9);
  assert.equal(view.health, "unavailable");
  assert.equal(view.error.includes("unit-key"), false);
  assert.match(view.error, /Bearer \[redacted\]/);
  assert.deepEqual(Object.keys(view).sort(), ["error", "health", "latency_ms", "ok"]);
});

test("toolListLeaksSecret flags token values only", () => {
  assert.equal(toolListLeaksSecret({ name: "web_search", token: "" }), false);
  assert.equal(toolListLeaksSecret({ name: "web_search", token: "secret-value" }), true);
  assert.equal(toolListLeaksSecret({ name: "web_search", api_key: "k" }), true);
});
