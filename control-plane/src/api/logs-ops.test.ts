import assert from "node:assert/strict";
import test from "node:test";
import { backoffDelay, parseSseBlock } from "./events-ops.ts";
import {
  applyFilters,
  asPublicLog,
  LIVE_CAP,
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
