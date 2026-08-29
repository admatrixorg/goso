import assert from "node:assert/strict";
import test from "node:test";
import {
  capText,
  groupErrors,
  pageLabel,
  parseTraceHash,
  publicAttrs,
  publicHasSecrets,
  rangeFrom,
  statusOf,
  tokenTotal,
  tracesHash,
  uniqueValues,
} from "./traces-ops.ts";

test("parseTraceHash reads id and empty list hash", () => {
  assert.equal(parseTraceHash("#traces/abc"), "abc");
  assert.equal(parseTraceHash("#traces"), "");
  assert.equal(parseTraceHash("#memory/x"), "");
  assert.equal(tracesHash("abc"), "#traces/abc");
  assert.equal(tracesHash(""), "#traces");
});

test("rangeFrom emits RFC3339 except all", () => {
  const now = Date.parse("2026-08-30T12:00:00Z");
  assert.equal(rangeFrom("all", now), undefined);
  assert.equal(rangeFrom("1h", now), "2026-08-30T11:00:00.000Z");
});

test("statusOf treats error text as error", () => {
  assert.equal(statusOf({ status: "ok" }), "ok");
  assert.equal(statusOf({ status: "error" }), "error");
  assert.equal(statusOf({ error: "boom" }), "error");
});

test("tokenTotal sums input and output", () => {
  assert.equal(tokenTotal({ trace_id: "t", input_tokens: 3, output_tokens: 5 }), 8);
  assert.equal(tokenTotal({ trace_id: "t" }), 0);
});

test("uniqueValues and groupErrors", () => {
  const rows = [
    { trace_id: "a", agent_id: "ag-1", channel: "telegram", error: "boom" },
    { trace_id: "b", agent_id: "ag-1", error: "boom" },
    { trace_id: "c", agent_id: "ag-2", channel: "telegram", error: "other" },
  ];
  assert.deepEqual(uniqueValues(rows, "agent_id"), ["ag-1", "ag-2"]);
  assert.deepEqual(uniqueValues(rows, "channel"), ["telegram"]);
  const g = groupErrors(rows);
  assert.equal(g[0].message, "boom");
  assert.equal(g[0].count, 2);
});

test("publicAttrs drops prompt and secrets; publicHasSecrets flags leaks", () => {
  const attrs = publicAttrs({
    prompt: "secret-prompt",
    api_key: "sk-live-abcdef",
    agent_id: "ag-1",
  });
  assert.equal(attrs?.agent_id, "ag-1");
  assert.equal(attrs?.prompt, undefined);
  assert.equal(attrs?.api_key, undefined);
  assert.equal(publicHasSecrets({ prompt: "x" }), true);
  assert.equal(publicHasSecrets({ agent_id: "ag-1", status: "ok" }), false);
});

test("capText and pageLabel", () => {
  assert.equal(capText("abcdef", 3), "abc…");
  assert.deepEqual(pageLabel(0, 20, 45), { from: 1, to: 20, pages: 3 });
  assert.deepEqual(pageLabel(20, 20, 45), { from: 21, to: 40, pages: 3 });
  assert.deepEqual(pageLabel(0, 20, 0), { from: 0, to: 0, pages: 1 });
});
