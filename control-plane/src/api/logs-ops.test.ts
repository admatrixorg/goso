import assert from "node:assert/strict";
import test from "node:test";
import { backoffDelay, classifyStreamConn, clearLocalRows, historyStreamProvenance, parseSseBlock, streamStartBlocked } from "./events-ops.ts";
import {
  applyFilters,
  asPublicLog,
  classifyLogsHistory,
  LIVE_CAP,
  logsActionsBlocked,
  logsFilteredEmpty,
  logsFiltersActive,
  mergeLive,
  publicHasSecrets,
  publicMessage,
  toggleLevel,
  uniqueComponents,
  type GatewayLog,
} from "./logs-ops.ts";

function row(over: Partial<GatewayLog> = {}): GatewayLog {
  return {
    seq: 1,
    ts: "2026-08-30T12:00:00Z",
    level: "info",
    component: "http",
    message: "GET /api/agents 200 3ms",
    ...over,
  };
}

test("asPublicLog drops credential keys and token shapes", () => {
  const leaked = asPublicLog(
    row({
      message: `{"path":"/api/agents","token":"super-secret","Authorization":"Bearer abcdefghijklmnop"}`,
    }),
  );
  assert.ok(leaked);
  assert.equal(leaked.message.includes("super-secret"), false);
  assert.equal(leaked.message.includes("Bearer abc"), false);
  assert.equal(leaked.message.includes("token"), false);
  const sk = asPublicLog(row({ message: "failed sk-abcdefghijk123" }));
  assert.ok(sk);
  assert.equal(sk.message.includes("sk-abcdefghijk123"), false);
  assert.ok(sk.message.includes("[redacted]"));
  const assign = asPublicLog(row({ message: "live-1 token=super-secret" }));
  assert.ok(assign);
  assert.equal(assign.message.includes("super-secret"), false);
});

test("publicHasSecrets flags leftover token shapes", () => {
  assert.equal(publicHasSecrets({ message: "GET /api/agents 200" }), false);
  assert.equal(publicHasSecrets({ message: "sk-abcdefghijk" }), true);
  assert.equal(publicHasSecrets({ message: "Bearer abcdefghijklmnop" }), true);
  assert.equal(publicHasSecrets({ message: `{"token":"abc"}` }), true);
});

test("mergeLive caps, filters, components, backoff", () => {
  assert.equal(backoffDelay(0), 1000);
  assert.equal(backoffDelay(4), 15000);
  let list: GatewayLog[] = [];
  for (let i = 0; i < LIVE_CAP + 5; i++) {
    list = mergeLive(list, row({ seq: i + 1, message: `n${i}` }));
  }
  assert.equal(list.length, LIVE_CAP);
  assert.equal(list[0].seq, LIVE_CAP + 5);
  const shown = applyFilters(list, { component: "http", q: "n1", levels: ["info"] });
  assert.ok(shown.length > 0);
  const none = applyFilters(list, { component: "llm" });
  assert.equal(none.length, 0);
  const comps = uniqueComponents(list, ["gateway"]);
  assert.ok(comps.includes("http"));
  assert.ok(comps.includes("gateway"));
  const levels = toggleLevel(["debug", "info", "warn", "error"], "debug");
  assert.deepEqual(levels, ["info", "warn", "error"]);
});

test("publicMessage caps and parseSseBlock", () => {
  const long = publicMessage("n".repeat(500));
  assert.ok(long.endsWith("…"));
  const sse = parseSseBlock("event: log\nid: 3\ndata: {\"seq\":3}\n");
  assert.equal(sse.event, "log");
  assert.equal(sse.id, "3");
});

test("history failure is not an empty live tail", () => {
  const perm = classifyLogsHistory({
    loading: false,
    loaded: false,
    error: new Error("401 unauthorized"),
    itemCount: 0,
  });
  assert.equal(perm.kind, "permission");
  assert.equal(perm.showEmpty, false);
  assert.equal(logsActionsBlocked(perm.kind), true);
  assert.equal(streamStartBlocked(perm.kind), true);
  assert.equal(historyStreamProvenance(perm.kind, "connecting"), "history");
  const ready = classifyLogsHistory({ loading: false, loaded: true, error: null, itemCount: 3 });
  assert.equal(historyStreamProvenance(ready.kind, "error"), "stream");
});

test("pause/resume and local clear do not delete history", () => {
  assert.equal(classifyStreamConn({ live: true, paused: true, conn: "live" }), "paused");
  assert.equal(classifyStreamConn({ live: true, paused: false, conn: "connecting" }), "connecting");
  assert.equal(backoffDelay(2), 4000);
  const history = [row({ seq: 1 }), row({ seq: 2, message: "ok" })];
  const live = [row({ seq: 9, message: "live-1" })];
  assert.equal(clearLocalRows(live).length, 0);
  assert.equal(history.length, 2);
});

test("filtered empty vs true empty and credential lines stay dropped", () => {
  const empty = classifyLogsHistory({ loading: false, loaded: true, error: null, itemCount: 0 });
  assert.equal(logsFilteredEmpty(empty, 0, 0, false), false);
  assert.equal(logsFilteredEmpty(empty, 0, 0, true), true);
  const ready = classifyLogsHistory({ loading: false, loaded: true, error: null, itemCount: 4 });
  assert.equal(logsFilteredEmpty(ready, 4, 0, true), true);
  assert.equal(logsFiltersActive({ q: "http" }), true);
  assert.equal(logsFiltersActive({ levels: ["debug", "info", "warn", "error"] }), false);
  const dropped = asPublicLog(row({ message: "password=super-secret token=abc" }));
  assert.ok(dropped);
  assert.equal(dropped.message.includes("super-secret"), false);
  assert.equal(publicHasSecrets({ message: "sk-abcdefghijk" }), true);
});
