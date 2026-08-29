import assert from "node:assert/strict";
import test from "node:test";
import {
  applyFilters,
  asPublicEvent,
  backoffDelay,
  eventKey,
  LIVE_CAP,
  mergeLive,
  parseDetail,
  parseSseBlock,
  publicHasSecrets,
  publicSummary,
  uniqueActors,
  type GatewayEvent,
} from "./events-ops.ts";

function row(over: Partial<GatewayEvent> = {}): GatewayEvent {
  return {
    seq: 1,
    trace_id: "tr-1",
    type: "connector",
    kind: "attempt",
    ts: "2026-08-30T12:00:00Z",
    summary: `{"query":"A"}`,
    ...over,
  };
}

test("asPublicEvent drops message/tool payload secrets", () => {
  const leaked = asPublicEvent(
    row({
      type: "message",
      summary: `{"action":"create","body":"secret-chat","arguments":{"token":"super-secret"},"from_agent_id":"a1"}`,
    }),
  );
  assert.ok(leaked);
  assert.equal(leaked.summary.includes("secret-chat"), false);
  assert.equal(leaked.summary.includes("super-secret"), false);
  assert.equal(leaked.summary.includes("body"), false);
  assert.ok(leaked.summary.includes("a1"));
});

test("publicHasSecrets flags token shapes and payload keys", () => {
  assert.equal(publicHasSecrets({ summary: `{"query":"A"}` }), false);
  assert.equal(publicHasSecrets({ summary: `{"body":"hi"}` }), true);
  assert.equal(publicHasSecrets({ summary: "sk-abcdefghijk" }), true);
  assert.equal(publicHasSecrets({ summary: "Bearer abcdefghijklmnop" }), true);
});

test("parseDetail is schema-safe and skips payload keys", () => {
  const d = parseDetail(
    row({
      type: "task",
      actor: "operator",
      agent_id: "ag1",
      summary: `{"action":"create","status":"todo","body":"nope","nested":{"x":1}}`,
    }),
  );
  const keys = d.map((x) => x.key);
  assert.ok(keys.includes("type"));
  assert.ok(keys.includes("action"));
  assert.ok(keys.includes("status"));
  assert.equal(keys.includes("body"), false);
  assert.equal(keys.includes("nested"), false);
});

test("mergeLive caps, dedupes, backoff, filters, actors", () => {
  assert.equal(backoffDelay(0), 1000);
  assert.equal(backoffDelay(1), 2000);
  assert.equal(backoffDelay(4), 15000);
  assert.equal(backoffDelay(9), 15000);
  let list: GatewayEvent[] = [];
  for (let i = 0; i < LIVE_CAP + 5; i++) {
    list = mergeLive(list, row({ seq: i + 1, trace_id: `tr-${i}` }));
  }
  assert.equal(list.length, LIVE_CAP);
  assert.equal(list[0].seq, LIVE_CAP + 5);
  const dup = mergeLive([row({ seq: 3 })], row({ seq: 3, summary: `{"query":"B"}` }));
  assert.equal(dup.length, 1);
  assert.equal(eventKey(row({ seq: 9 })), "seq:9");
  const filtered = applyFilters(
    [row({ type: "agent", actor: "operator", agent_id: "ag1" }), row({ type: "team", team_id: "tm1", actor: "operator" })],
    { type: "agent", actor: "ag1" },
  );
  assert.equal(filtered.length, 1);
  const actors = uniqueActors([row({ actor: "operator", agent_id: "ag1" }), row({ team_id: "tm1" })]);
  assert.ok(actors.includes("operator"));
  assert.ok(actors.includes("ag1"));
  assert.ok(actors.includes("tm1"));
});

test("parseSseBlock and publicSummary cap", () => {
  const b = parseSseBlock("id: 12\nevent: ops\ndata: {\"kind\":\"success\"}");
  assert.equal(b.id, "12");
  assert.equal(b.event, "ops");
  assert.ok(b.data.includes("success"));
  const long = publicSummary("x".repeat(500));
  assert.ok(long.endsWith("…"));
  assert.ok(long.length <= 401);
});
