import assert from "node:assert/strict";
import test from "node:test";
import {
  approvalLabel,
  asPublic,
  asPublicList,
  canResolve,
  isExpired,
  listHasSecrets,
  publicHasSecrets,
  type Approval,
} from "./approvals-ops.ts";

function row(over: Partial<Approval> = {}): Approval {
  return {
    id: "appr-1",
    approval_id: "appr-1",
    kind: "execution",
    requester: "agent:agt_1",
    agent_id: "agt_1",
    connector: "zalocrm",
    tool: "message_send",
    arg_preview: '{"contact_id":"1"}',
    risk: "high",
    status: "pending",
    expires_at: "2099-01-01T00:00:00Z",
    stale: false,
    ...over,
  };
}

test("asPublic drops secret-shaped rows and keeps metadata", () => {
  const rows = asPublic([
    row(),
    row({ id: "appr-2", token: "sk-live-abcdefgh" } as never),
    row({ id: "appr-3", args: { token: "x" } } as never),
  ]);
  assert.equal(rows.length, 1);
  assert.equal(rows[0].id, "appr-1");
  assert.equal(rows[0].kind, "execution");
  assert.equal(rows[0].arg_preview, '{"contact_id":"1"}');
  assert.equal("args" in rows[0], false);
  assert.equal(publicHasSecrets(rows[0]), false);
});

test("publicHasSecrets flags args/token payloads, not listing metadata", () => {
  assert.equal(publicHasSecrets(row()), false);
  assert.equal(publicHasSecrets({ id: "appr-1", token: "abc" }), true);
  assert.equal(publicHasSecrets({ id: "appr-1", args: {} }), true);
  assert.equal(publicHasSecrets({ id: "appr-1", arg_preview: "sk-live-abcdefgh" }), true);
  assert.equal(listHasSecrets({ approvals: [row()] }), false);
  assert.equal(listHasSecrets({ approvals: [{ id: "appr-1", secret: "x" }] }), true);
});

test("asPublicList counts pending and drops leaks", () => {
  const list = asPublicList({
    approvals: [row(), row({ id: "appr-2", status: "expired", stale: true }), row({ id: "appr-3", token: "x" } as never)],
    pending: 99,
  });
  assert.equal(list.approvals.length, 2);
  assert.equal(list.pending, 1);
});

test("canResolve and isExpired", () => {
  assert.equal(canResolve(row()), true);
  assert.equal(canResolve(row({ stale: true })), true);
  assert.equal(canResolve(row({ status: "expired", stale: true })), false);
  assert.equal(canResolve(row({ status: "approved" })), false);
  assert.equal(isExpired(row({ status: "expired" })), true);
  assert.equal(isExpired({ status: "pending", expires_at: "2000-01-01T00:00:00Z" }), true);
  assert.equal(isExpired({ status: "pending", expires_at: "2099-01-01T00:00:00Z" }), false);
});

test("approvalLabel", () => {
  assert.equal(approvalLabel(row()), "zalocrm/message_send");
  assert.equal(approvalLabel({ id: "appr-1", tool: "write_file", connector: "" }), "write_file");
  assert.equal(approvalLabel({ id: "appr-1", tool: "", connector: "" }), "appr-1");
});
