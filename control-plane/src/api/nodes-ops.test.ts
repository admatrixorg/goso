import assert from "node:assert/strict";
import test from "node:test";
import { classifyPageState, inventoryBlocksMutation, listMetaCount } from "./page-state.ts";
import { asPublic, formatWhen, nodeConfirmMatch, nodeInventoryCount, nodeLabel, publicHasSecrets } from "./nodes-ops.ts";
import type { NodeDevice } from "./nodes.ts";

function row(over: Partial<NodeDevice> = {}): NodeDevice {
  return {
    id: "nd_1",
    display: "ops-laptop",
    kind: "dashboard",
    status: "pending",
    health: "pending",
    requested_at: "2026-08-30T12:00:00Z",
    expires_at: "2026-08-30T12:10:00Z",
    ...over,
  };
}

test("asPublic drops secret-shaped rows and keeps metadata", () => {
  const rows = asPublic([
    row(),
    row({ id: "nd_2", token: "wh_full" } as never),
  ]);
  assert.equal(rows.length, 1);
  assert.equal(rows[0].id, "nd_1");
  assert.equal(rows[0].display, "ops-laptop");
  assert.equal(publicHasSecrets(rows[0]), false);
});

test("publicHasSecrets flags token/code payloads, not listing metadata", () => {
  assert.equal(publicHasSecrets(row()), false);
  assert.equal(publicHasSecrets({ id: "nd_1", token: "abc" }), true);
  assert.equal(publicHasSecrets({ id: "nd_1", code: "1234" }), true);
  assert.equal(publicHasSecrets({ id: "nd_1", content: "hello" }), true);
  assert.equal(publicHasSecrets({ id: "nd_1", display: "sk-live-abcdefgh" }), true);
});

test("nodeConfirmMatch accepts id or display", () => {
  const n = { id: "nd_1", display: "ops-laptop" };
  assert.equal(nodeConfirmMatch("nd_1", n), true);
  assert.equal(nodeConfirmMatch("ops-laptop", n), true);
  assert.equal(nodeConfirmMatch(" nd_1 ", n), true);
  assert.equal(nodeConfirmMatch("", n), false);
  assert.equal(nodeConfirmMatch("other", n), false);
});

test("permission never claims pending or paired empty", () => {
  const pending = asPublic([row()]);
  const paired: NodeDevice[] = [];
  const count = nodeInventoryCount(pending, paired);
  const s = classifyPageState({
    loading: false,
    loaded: false,
    error: new Error("401 unauthorized"),
    itemCount: count,
  });
  assert.equal(s.kind, "permission");
  assert.equal(s.showEmpty, false);
  assert.equal(listMetaCount(s.kind, pending.length), null);
  assert.equal(listMetaCount(s.kind, paired.length), null);
  assert.equal(inventoryBlocksMutation(s.kind), true);
});

test("true empty pending and paired only after successful load", () => {
  const s = classifyPageState({
    loading: false,
    loaded: true,
    error: null,
    itemCount: nodeInventoryCount([], []),
  });
  assert.equal(s.kind, "empty");
  assert.equal(s.showEmpty, true);
});

test("nodeLabel and formatWhen", () => {
  assert.equal(nodeLabel({ id: "nd_1", display: "ops-laptop" }), "ops-laptop");
  assert.equal(nodeLabel({ id: "nd_1", display: "  " }), "nd_1");
  assert.equal(formatWhen("", "n/a"), "n/a");
  assert.equal(formatWhen("not-a-date", "n/a"), "not-a-date");
  const shown = formatWhen("2026-08-30T12:00:00Z", "n/a");
  assert.notEqual(shown, "n/a");
  assert.ok(shown.includes("2026") || shown.includes("30"));
});
