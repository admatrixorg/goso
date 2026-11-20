import assert from "node:assert/strict";
import test from "node:test";
import { confirmNamed, namedConfirmTarget, typedConfirm } from "./confirm.ts";

test("namedConfirmTarget requires the exact trimmed name", () => {
  assert.equal(namedConfirmTarget("Ops", "Ops"), true);
  assert.equal(namedConfirmTarget("Ops", " Ops "), true);
  assert.equal(namedConfirmTarget("Ops", "ops"), false);
  assert.equal(namedConfirmTarget("Ops", "Support"), false);
  assert.equal(namedConfirmTarget("  ", "  "), false);
});

test("confirmNamed only proceeds on true", () => {
  assert.equal(confirmNamed("Delete agent “sales”?", () => true), true);
  assert.equal(confirmNamed("Delete agent “sales”?", () => false), false);
});

test("typedConfirm distinguishes cancel, mismatch, and ok", () => {
  assert.equal(typedConfirm("Ops", null), "cancel");
  assert.equal(typedConfirm("Ops", undefined), "cancel");
  assert.equal(typedConfirm("Ops", "ops"), "mismatch");
  assert.equal(typedConfirm("Ops", "Ops"), "ok");
  assert.equal(typedConfirm("Ops", " Ops "), "ok");
});
