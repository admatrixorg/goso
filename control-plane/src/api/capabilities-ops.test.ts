import assert from "node:assert/strict";
import test from "node:test";
import {
  MCP_UNAVAILABLE,
  SKILL_UNAVAILABLE,
  classifyToolView,
  connectorFormError,
  connectorRowLeaksSecret,
  connectorTestReady,
  cronCreateBlocked,
  disposeOneTimeSecrets,
  filterByQuery,
  isHttpEndpoint,
  isRouteUnsupported,
  parseCronSpec,
  publicConnector,
  safeCredentialRef,
  skillNameOk,
  toolViewBlocksMutation,
  ttsBlocksMutation,
  ttsItemCount,
} from "./capabilities-ops.ts";
import { classifyPageState, inventoryBlocksMutation } from "./page-state.ts";
import { connectorWriteBody } from "./function-ops.ts";
import { hideCopiedSecret } from "./webhooks-ops.ts";

test("tool view: permission on agent inventory never claims empty or pick-agent", () => {
  const kind = classifyToolView({
    agentsLoading: false,
    agentsLoaded: false,
    agentsError: new Error('401 {"error":"unauthorized"}'),
    agentCount: 0,
    agentId: "",
    toolsLoading: false,
    toolsLoaded: false,
    toolsError: null,
    toolCount: 0,
  });
  assert.equal(kind, "permission");
  assert.equal(toolViewBlocksMutation(kind), true);
});

test("tool view: agent inventory error is not 0 tools", () => {
  const kind = classifyToolView({
    agentsLoading: false,
    agentsLoaded: false,
    agentsError: new Error("non-JSON response"),
    agentCount: 0,
    agentId: "",
    toolsLoading: false,
    toolsLoaded: false,
    toolsError: null,
    toolCount: 0,
  });
  assert.equal(kind, "error");
  assert.notEqual(kind, "empty");
  assert.notEqual(kind, "no_selection");
});

test("tool view: loaded zero agents is no_agent, not no_selection", () => {
  const kind = classifyToolView({
    agentsLoading: false,
    agentsLoaded: true,
    agentsError: null,
    agentCount: 0,
    agentId: "",
    toolsLoading: false,
    toolsLoaded: false,
    toolsError: null,
    toolCount: 0,
  });
  assert.equal(kind, "no_agent");
});

test("tool view: agents ready without selection is no_selection", () => {
  const kind = classifyToolView({
    agentsLoading: false,
    agentsLoaded: true,
    agentsError: null,
    agentCount: 2,
    agentId: "",
    toolsLoading: false,
    toolsLoaded: false,
    toolsError: null,
    toolCount: 0,
  });
  assert.equal(kind, "no_selection");
  assert.equal(toolViewBlocksMutation(kind), true);
});

test("tool view: 404 on tools route is unsupported", () => {
  const kind = classifyToolView({
    agentsLoading: false,
    agentsLoaded: true,
    agentsError: null,
    agentCount: 1,
    agentId: "ag1",
    toolsLoading: false,
    toolsLoaded: false,
    toolsError: new Error('404 {"error":"not found"}'),
    toolCount: 0,
  });
  assert.equal(kind, "unsupported");
  assert.equal(isRouteUnsupported(new Error("404 missing")), true);
  assert.equal(isRouteUnsupported(new Error("500 boom")), false);
});

test("tool view: true empty vs filtered empty", () => {
  const empty = classifyToolView({
    agentsLoading: false,
    agentsLoaded: true,
    agentsError: null,
    agentCount: 1,
    agentId: "ag1",
    toolsLoading: false,
    toolsLoaded: true,
    toolsError: null,
    toolCount: 0,
  });
  assert.equal(empty, "empty");
  const filtered = classifyToolView({
    agentsLoading: false,
    agentsLoaded: true,
    agentsError: null,
    agentCount: 1,
    agentId: "ag1",
    toolsLoading: false,
    toolsLoaded: true,
    toolsError: null,
    toolCount: 3,
    visibleCount: 0,
  });
  assert.equal(filtered, "filtered_empty");
});

test("tool view: stale keeps last-known tools", () => {
  const kind = classifyToolView({
    agentsLoading: false,
    agentsLoaded: true,
    agentsError: null,
    agentCount: 1,
    agentId: "ag1",
    toolsLoading: false,
    toolsLoaded: true,
    toolsError: new Error("502 upstream"),
    toolCount: 4,
  });
  assert.equal(kind, "stale");
  assert.equal(toolViewBlocksMutation(kind), false);
});

test("cron spec: interval and five-field UTC; once is unavailable", () => {
  assert.deepEqual(parseCronSpec("every:1h"), { ok: true, kind: "interval" });
  assert.deepEqual(parseCronSpec("every:15m"), { ok: true, kind: "interval" });
  assert.deepEqual(parseCronSpec("0 * * * *"), { ok: true, kind: "five" });
  assert.deepEqual(parseCronSpec("once"), { ok: false, reason: "once" });
  assert.deepEqual(parseCronSpec("once:tomorrow"), { ok: false, reason: "once" });
  assert.deepEqual(parseCronSpec("at:09:00"), { ok: false, reason: "once" });
  assert.deepEqual(parseCronSpec(""), { ok: false, reason: "empty" });
  assert.deepEqual(parseCronSpec("every:0m"), { ok: false, reason: "invalid" });
  assert.deepEqual(parseCronSpec("every:30s"), { ok: false, reason: "invalid" });
  assert.deepEqual(parseCronSpec("* * * * * *"), { ok: false, reason: "invalid" });
});

test("cron create blocked when jobs permission or sessions dependency", () => {
  const perm = classifyPageState({
    loading: false,
    loaded: false,
    error: new Error("401 unauthorized"),
    itemCount: 0,
  });
  const readyJobs = classifyPageState({ loading: false, loaded: true, error: null, itemCount: 0 });
  const sessFail = classifyPageState({
    loading: false,
    loaded: false,
    error: new Error("non-JSON response"),
    itemCount: 0,
  });
  const sessOk = classifyPageState({ loading: false, loaded: true, error: null, itemCount: 2 });
  const sessEmpty = classifyPageState({ loading: false, loaded: true, error: null, itemCount: 0 });
  assert.equal(cronCreateBlocked(perm, sessOk, 2), true);
  assert.equal(cronCreateBlocked(readyJobs, sessFail, 0), true);
  assert.equal(cronCreateBlocked(readyJobs, sessEmpty, 0), true);
  assert.equal(cronCreateBlocked(readyJobs, sessOk, 2), false);
});

test("connector transport validation", () => {
  assert.equal(connectorFormError({ name: "", transport: "http", endpoint: "http://127.0.0.1:9" }), "needName");
  assert.equal(connectorFormError({ name: "crm", transport: "nope", endpoint: "http://127.0.0.1:9" }), "badTransport");
  assert.equal(connectorFormError({ name: "crm", transport: "sse", endpoint: "not-a-url" }), "needUrl");
  assert.equal(connectorFormError({ name: "crm", transport: "http", endpoint: "" }), "needUrl");
  assert.equal(connectorFormError({ name: "local", transport: "stdio", endpoint: "" }), "needCommand");
  assert.equal(connectorFormError({ name: "crm", transport: "mcp-http", endpoint: "http://127.0.0.1:9/sse" }), null);
  assert.equal(connectorFormError({ name: "local", transport: "mcp-stdio", endpoint: "npx -y demo" }), null);
  assert.equal(isHttpEndpoint("http://127.0.0.1:9"), true);
  assert.equal(isHttpEndpoint("/usr/bin/node"), false);
});

test("connector write body omits blank token; public connector drops secrets", () => {
  const body = connectorWriteBody({ name: "crm", transport: "sse", endpoint: " http://127.0.0.1:9 ", token: "  " });
  assert.equal("token" in body, false);
  const leaked = publicConnector({ name: "crm", transport: "http", endpoint: "http://x", token: "secret-value", enabled: true });
  assert.equal(leaked, null);
  const pub = publicConnector({
    name: "crm",
    transport: "http",
    endpoint: "http://127.0.0.1:9",
    token_set: true,
    env_owned: false,
    enabled: true,
  });
  assert.equal(pub?.name, "crm");
  assert.equal(pub && "token" in pub, false);
  assert.equal(connectorRowLeaksSecret({ name: "crm", token: "x" }), true);
  assert.equal(connectorTestReady({ name: "crm", transport: "http", endpoint: "http://127.0.0.1:9" }), true);
  assert.equal(connectorTestReady({ name: "crm", transport: "http", endpoint: "" }), false);
  assert.equal(safeCredentialRef("GOSO_CRM_TOKEN"), "GOSO_CRM_TOKEN");
  assert.equal(safeCredentialRef("secret:box"), undefined);
  assert.equal(safeCredentialRef("sk-leaked"), undefined);
});

test("one-time webhook secrets leave UI memory after copy and dispose", () => {
  const last = { id: "w1", token_prefix: "wh_ab", token: "wh_secret", hmac_key: "hmac_secret" };
  const afterToken = hideCopiedSecret(last, "token");
  assert.equal(afterToken.token, undefined);
  assert.equal(afterToken.hmac_key, "hmac_secret");
  const gone = disposeOneTimeSecrets(afterToken);
  assert.equal(gone?.token, undefined);
  assert.equal(gone?.hmac_key, undefined);
  assert.equal(gone?.token_prefix, "wh_ab");
  assert.equal(disposeOneTimeSecrets(null), null);
});

test("skill name and named archive confirm message interpolation target", () => {
  assert.equal(skillNameOk("demo_skill"), true);
  assert.equal(skillNameOk("Bad Name"), false);
  assert.equal(skillNameOk(""), false);
  assert.deepEqual(SKILL_UNAVAILABLE.slice().sort(), ["bulk", "deps", "edit", "enable", "install", "rescan", "status"]);
  assert.ok(MCP_UNAVAILABLE.includes("credential_clear"));
});

test("filterByQuery is distinct from true empty", () => {
  const rows = [{ name: "alpha" }, { name: "beta" }];
  assert.equal(filterByQuery(rows, "alp", (r) => r.name).length, 1);
  assert.equal(filterByQuery(rows, "zzz", (r) => r.name).length, 0);
  assert.equal(filterByQuery([], "alp", (r) => r.name).length, 0);
});

test("tts permission is not not-configured; mutations stay closed", () => {
  const perm = classifyPageState({
    loading: false,
    loaded: false,
    error: new Error('401 {"error":"unauthorized"}'),
    itemCount: ttsItemCount(false),
  });
  assert.equal(perm.kind, "permission");
  assert.equal(perm.showEmpty, false);
  assert.equal(ttsBlocksMutation(perm.kind), true);
  assert.equal(ttsBlocksMutation("ready"), false);
  const loaded = classifyPageState({
    loading: false,
    loaded: true,
    error: null,
    itemCount: ttsItemCount(true),
  });
  assert.equal(loaded.kind, "ready");
  assert.equal(inventoryBlocksMutation(loaded.kind), false);
});
