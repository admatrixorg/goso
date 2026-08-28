import assert from "node:assert/strict";
import test from "node:test";
import { healthKind } from "./health.ts";

test("200 + ok maps to connected", () => {
  assert.equal(healthKind(200, true), "connected");
});

test("non-200 or 200 without ok maps to degraded", () => {
  assert.equal(healthKind(502, false), "degraded");
  assert.equal(healthKind(500, true), "degraded");
  assert.equal(healthKind(200, false), "degraded");
});

test("401 and 403 map to unauthorized", () => {
  assert.equal(healthKind(401, false), "unauthorized");
  assert.equal(healthKind(403, true), "unauthorized");
});

test("network failure maps to offline", () => {
  assert.equal(healthKind(0, false), "offline");
  assert.equal(healthKind(-1, false), "offline");
});
