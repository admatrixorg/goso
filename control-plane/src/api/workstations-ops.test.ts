import assert from "node:assert/strict";
import test from "node:test";
import {
  asPublic,
  asPublicTest,
  formatWhen,
  identityError,
  looksLikeKey,
  publicHasSecrets,
  writeBody,
  wsConfirmMatch,
  wsLabel,
} from "./workstations-ops.ts";
import type { Workstation } from "./workstations.ts";

function row(over: Partial<Workstation> = {}): Workstation {
  return {
    id: "ws_1",
    display: "lab",
    backend: "ssh",
    host: "10.0.0.8",
    port: 22,
    user: "ops",
    identity_ref: "~/.ssh/id_ed25519",
    identity_set: true,
    agent_id: "ag_1",
    health: "unknown",
    ...over,
  };
}

test("asPublic keeps path identity and drops key-shaped rows", () => {
  const rows = asPublic([
    row(),
    row({ id: "ws_2", private_key: "AAAA" } as never),
    row({ id: "ws_3", identity_ref: "-----BEGIN OPENSSH PRIVATE KEY-----" }),
  ]);
  assert.equal(rows.length, 1);
  assert.equal(rows[0].id, "ws_1");
  assert.equal(rows[0].identity_ref, "~/.ssh/id_ed25519");
  assert.equal(rows[0].identity_set, true);
  assert.equal(publicHasSecrets(rows[0]), false);
});

test("publicHasSecrets flags keys, not listing metadata", () => {
  assert.equal(publicHasSecrets(row()), false);
  assert.equal(publicHasSecrets({ id: "ws_1", private_key: "abc" }), true);
  assert.equal(publicHasSecrets({ id: "ws_1", password: "x" }), true);
  assert.equal(publicHasSecrets({ id: "ws_1", identity_ref: "BEGIN RSA PRIVATE KEY" }), true);
  assert.equal(publicHasSecrets({ id: "ws_1", display: "sk-live-abcdefgh" }), true);
});

test("looksLikeKey accepts paths and named refs", () => {
  assert.equal(looksLikeKey("~/.ssh/id_ed25519"), false);
  assert.equal(looksLikeKey("/home/ops/.ssh/ops"), false);
  assert.equal(looksLikeKey("ssh:ops-laptop"), false);
  assert.equal(looksLikeKey("-----BEGIN OPENSSH PRIVATE KEY-----"), true);
  assert.equal(identityError("~/.ssh/id_ed25519"), null);
  assert.equal(identityError("-----BEGIN OPENSSH PRIVATE KEY-----"), "ws.keyMaterial");
  assert.equal(identityError("ssh://evil"), "ws.needPath");
});

test("wsConfirmMatch accepts id or display", () => {
  const n = { id: "ws_1", display: "lab" };
  assert.equal(wsConfirmMatch("ws_1", n), true);
  assert.equal(wsConfirmMatch("lab", n), true);
  assert.equal(wsConfirmMatch(" ws_1 ", n), true);
  assert.equal(wsConfirmMatch("", n), false);
  assert.equal(wsConfirmMatch("other", n), false);
});

test("wsLabel, formatWhen, writeBody, asPublicTest", () => {
  assert.equal(wsLabel({ id: "ws_1", display: "lab" }), "lab");
  assert.equal(wsLabel({ id: "ws_1", display: "  " }), "ws_1");
  assert.equal(formatWhen("", "n/a"), "n/a");
  assert.equal(formatWhen("not-a-date", "n/a"), "not-a-date");
  const shown = formatWhen("2026-08-30T12:00:00Z", "n/a");
  assert.notEqual(shown, "n/a");
  assert.ok(shown.includes("2026") || shown.includes("30"));
  const body = writeBody({
    display: " lab ",
    backend: "ssh",
    host: "10.0.0.8",
    port: "22",
    user: "ops",
    identity_ref: "~/.ssh/id_ed25519",
    agent_id: "ag_1",
  });
  assert.equal(body.display, "lab");
  assert.equal(body.port, 22);
  assert.equal(body.identity_ref, "~/.ssh/id_ed25519");
  const tr = asPublicTest({
    ok: true,
    health: "ok",
    summary: "ssh config valid",
    backend: "ssh",
    host: "10.0.0.8",
    port: 22,
    identity_set: true,
  });
  assert.equal(tr?.ok, true);
  assert.equal(tr?.identity_set, true);
  assert.equal(asPublicTest({ ok: true, health: "ok", summary: "x", backend: "ssh", host: "h", port: 22, identity_set: false, private_key: "x" } as never), null);
});
