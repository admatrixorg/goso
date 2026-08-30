import assert from "node:assert/strict";
import test from "node:test";
import {
  capText,
  classifyTraceDetail,
  classifyTracesList,
  groupErrors,
  pageLabel,
  parseTraceHash,
  publicAttrs,
  publicHasSecrets,
  rangeFrom,
  statusOf,
  tokenTotal,
  tracesActionsBlocked,
  tracesFilteredEmpty,
  tracesFiltersActive,
  tracesHash,
  tracesTrueEmpty,
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

test("permission and error never claim true-empty traces", () => {
  const perm = classifyTracesList({
    loading: false,
    loaded: false,
    error: new Error("401 unauthorized"),
    itemCount: 0,
  });
  assert.equal(perm.kind, "permission");
  assert.equal(perm.showEmpty, false);
  assert.equal(tracesTrueEmpty(perm, false), false);
  assert.equal(tracesFilteredEmpty(perm, true), false);
  assert.equal(tracesActionsBlocked(perm.kind), true);
  const err = classifyTracesList({
    loading: false,
    loaded: false,
    error: new Error("502 upstream"),
    itemCount: 0,
  });
  assert.equal(err.kind, "error");
  assert.equal(err.showEmpty, false);
});

test("filtered empty is distinct from true empty", () => {
  const empty = classifyTracesList({ loading: false, loaded: true, error: null, itemCount: 0 });
  assert.equal(tracesTrueEmpty(empty, false), true);
  assert.equal(tracesFilteredEmpty(empty, true), true);
  assert.equal(tracesTrueEmpty(empty, true), false);
  assert.equal(tracesFiltersActive({ q: "boom", range: "all" }), true);
  assert.equal(tracesFiltersActive({ range: "all" }), false);
});

test("stale keeps last-known traces; 401 still hides them", () => {
  const stale = classifyTracesList({
    loading: false,
    loaded: true,
    error: new Error("502"),
    itemCount: 3,
  });
  assert.equal(stale.kind, "stale");
  assert.equal(stale.showItems, true);
  const perm = classifyTracesList({
    loading: false,
    loaded: true,
    error: new Error("401 unauthorized"),
    itemCount: 3,
  });
  assert.equal(perm.kind, "permission");
  assert.equal(perm.showItems, false);
});

test("detail failure is not inventory empty", () => {
  assert.equal(classifyTraceDetail({ selectedId: "", loading: false, error: null, hasDetail: false }), "idle");
  assert.equal(classifyTraceDetail({ selectedId: "t1", loading: true, error: null, hasDetail: false }), "loading");
  assert.equal(
    classifyTraceDetail({ selectedId: "t1", loading: false, error: new Error("401 unauthorized"), hasDetail: false }),
    "permission",
  );
  assert.equal(
    classifyTraceDetail({ selectedId: "t1", loading: false, error: new Error("404 missing"), hasDetail: false }),
    "error",
  );
  assert.equal(classifyTraceDetail({ selectedId: "t1", loading: false, error: null, hasDetail: false }), "missing");
  assert.equal(classifyTraceDetail({ selectedId: "t1", loading: false, error: null, hasDetail: true }), "ready");
  const list = classifyTracesList({ loading: false, loaded: true, error: null, itemCount: 4 });
  assert.equal(list.kind, "ready");
  assert.equal(tracesTrueEmpty(list, false), false);
});
