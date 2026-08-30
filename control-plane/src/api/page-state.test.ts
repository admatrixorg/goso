import assert from "node:assert/strict";
import test from "node:test";
import { classifyPageState, formatStaleAt, isPermissionError } from "./page-state.ts";

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
