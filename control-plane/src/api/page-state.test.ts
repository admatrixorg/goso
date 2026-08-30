import assert from "node:assert/strict";
import test from "node:test";
import {
  classifyPageState,
  clampPageOffset,
  formatStaleAt,
  inventoryBlocksMutation,
  isFilteredEmpty,
  isPermissionError,
  listMetaCount,
  pageSlice,
} from "./page-state.ts";

test("loading first fetch is loading, never empty", () => {
  const s = classifyPageState({ loading: true, loaded: false, error: null, itemCount: 0 });
  assert.equal(s.kind, "loading");
  assert.equal(s.showEmpty, false);
  assert.equal(s.showItems, false);
});

test("true empty only after successful load with zero items", () => {
  const s = classifyPageState({ loading: false, loaded: true, error: null, itemCount: 0 });
  assert.equal(s.kind, "empty");
  assert.equal(s.showEmpty, true);
  assert.equal(s.showItems, false);
});

test("permission never claims empty even with zero count", () => {
  const s = classifyPageState({
    loading: false,
    loaded: false,
    error: new Error('401 {"error":"unauthorized"}'),
    itemCount: 0,
  });
  assert.equal(s.kind, "permission");
  assert.equal(s.showEmpty, false);
  assert.equal(s.showItems, false);
  assert.equal(isPermissionError(new Error("403 forbidden")), true);
  assert.equal(isPermissionError(new Error("500 boom")), false);
});

test("generic error never claims empty", () => {
  const s = classifyPageState({
    loading: false,
    loaded: false,
    error: new Error("non-JSON response"),
    itemCount: 0,
  });
  assert.equal(s.kind, "error");
  assert.equal(s.showEmpty, false);
});

test("stale keeps last-known items and never shows empty", () => {
  const s = classifyPageState({
    loading: false,
    loaded: true,
    error: new Error("502 upstream"),
    itemCount: 3,
    keepStale: true,
  });
  assert.equal(s.kind, "stale");
  assert.equal(s.showItems, true);
  assert.equal(s.showEmpty, false);
});

test("stale without items falls through to error/permission", () => {
  const s = classifyPageState({
    loading: false,
    loaded: true,
    error: new Error("502"),
    itemCount: 0,
    keepStale: true,
  });
  assert.equal(s.kind, "error");
  assert.equal(s.showEmpty, false);
});

test("permission wins over keepStale so 401 never enables mutations", () => {
  const s = classifyPageState({
    loading: false,
    loaded: true,
    error: new Error('401 {"error":"unauthorized"}'),
    itemCount: 4,
    keepStale: true,
  });
  assert.equal(s.kind, "permission");
  assert.equal(s.showItems, false);
  assert.equal(s.showEmpty, false);
  assert.equal(inventoryBlocksMutation(s.kind), true);
});

test("in-flight refresh is loading, not stale, while a prior 502 is still set", () => {
  const s = classifyPageState({
    loading: true,
    loaded: true,
    error: new Error("502 upstream"),
    itemCount: 3,
    keepStale: true,
  });
  assert.equal(s.kind, "loading");
  assert.equal(s.showItems, true);
  assert.equal(s.showEmpty, false);
});

test("ready shows items", () => {
  const s = classifyPageState({ loading: false, loaded: true, error: null, itemCount: 2 });
  assert.equal(s.kind, "ready");
  assert.equal(s.showItems, true);
  assert.equal(s.showEmpty, false);
});

test("refresh loading keeps items and does not claim empty", () => {
  const s = classifyPageState({ loading: true, loaded: true, error: null, itemCount: 4 });
  assert.equal(s.kind, "loading");
  assert.equal(s.showItems, true);
  assert.equal(s.showEmpty, false);
});

test("formatStaleAt returns empty for missing timestamps", () => {
  assert.equal(formatStaleAt(""), "");
  assert.equal(formatStaleAt(undefined), "");
  assert.match(formatStaleAt("2026-08-30T03:04:05Z", "en"), /2026/);
});

test("inventoryBlocksMutation only for error and permission", () => {
  assert.equal(inventoryBlocksMutation("error"), true);
  assert.equal(inventoryBlocksMutation("permission"), true);
  assert.equal(inventoryBlocksMutation("loading"), false);
  assert.equal(inventoryBlocksMutation("empty"), false);
  assert.equal(inventoryBlocksMutation("ready"), false);
  assert.equal(inventoryBlocksMutation("stale"), false);
});

test("filtered empty is distinct from true empty", () => {
  const ready = classifyPageState({ loading: false, loaded: true, error: null, itemCount: 4 });
  assert.equal(isFilteredEmpty(ready, 4, 0), true);
  assert.equal(isFilteredEmpty(ready, 4, 1), false);
  const empty = classifyPageState({ loading: false, loaded: true, error: null, itemCount: 0 });
  assert.equal(isFilteredEmpty(empty, 0, 0), false);
  const perm = classifyPageState({
    loading: false,
    loaded: false,
    error: new Error("401 unauthorized"),
    itemCount: 0,
  });
  assert.equal(isFilteredEmpty(perm, 0, 0), false);
});

test("listMetaCount never reports zero during blocking states", () => {
  assert.equal(listMetaCount("permission", 0), null);
  assert.equal(listMetaCount("error", 0), null);
  assert.equal(listMetaCount("loading", 0), null);
  assert.equal(listMetaCount("empty", 0), 0);
  assert.equal(listMetaCount("ready", 3), 3);
  assert.equal(listMetaCount("stale", 2), 2);
});

test("clampPageOffset recovers after the last page shrinks", () => {
  assert.equal(clampPageOffset(0, 20, 20), 0);
  assert.equal(clampPageOffset(5, 20, 20), 0);
  assert.equal(clampPageOffset(45, 40, 20), 40);
  assert.equal(clampPageOffset(21, 40, 20), 20);
  assert.equal(clampPageOffset(21, 25, 20), 20);
  assert.equal(pageSlice(["a", "b", "c", "d"], 20, 2).join(), "c,d");
  assert.equal(pageSlice(["a", "b", "c"], 0, 2).join(), "a,b");
});
