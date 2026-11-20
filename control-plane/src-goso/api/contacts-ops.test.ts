import assert from "node:assert/strict";
import test from "node:test";
import {
  asPublic,
  channelIdsLine,
  filterContacts,
  lastSourceId,
  mergeConfirmMatch,
  mergePair,
  pageOf,
  publicHasSecrets,
  swapMergePair,
  undoConfirmMatch,
} from "./contacts-ops.ts";
import type { Contact } from "./contacts.ts";

function row(over: Partial<Contact> = {}): Contact {
  return {
    id: "ct_1",
    display: "telegram:111",
    kind: "user",
    channel: "telegram",
    dest: "111",
    identifiers: [{ channel: "telegram", dest: "111", kind: "user", permission: "direct" }],
    count: 1,
    ...over,
  };
}

test("asPublic drops secret-shaped rows and keeps identifiers", () => {
  const rows = asPublic([
    row(),
    row({ id: "ct_2", dest: "222", token: "wh_full" } as never),
  ]);
  assert.equal(rows.length, 1);
  assert.equal(rows[0].id, "ct_1");
  assert.equal(rows[0].identifiers.length, 1);
  assert.equal(publicHasSecrets(rows[0]), false);
});

test("publicHasSecrets flags token/code payloads, not listing metadata", () => {
  assert.equal(publicHasSecrets(row()), false);
  assert.equal(publicHasSecrets({ id: "ct_1", token: "abc" }), true);
  assert.equal(publicHasSecrets({ id: "ct_1", code: "1234" }), true);
  assert.equal(publicHasSecrets({ id: "ct_1", content: "hello" }), true);
  assert.equal(publicHasSecrets({ id: "ct_1", dest: "sk-live-abcdefgh" }), true);
  assert.equal(publicHasSecrets({ id: "ct_1", identifiers: [{ channel: "tg", dest: "1", bot_token: "x" }] }), true);
});

test("mergeConfirmMatch accepts source id, target id, dest, or source>target", () => {
  const target = { id: "ct_1" };
  const source = { id: "ct_2", dest: "222" };
  assert.equal(mergeConfirmMatch("ct_2", target, source), true);
  assert.equal(mergeConfirmMatch("ct_1", target, source), true);
  assert.equal(mergeConfirmMatch("222", target, source), true);
  assert.equal(mergeConfirmMatch("ct_2>ct_1", target, source), true);
  assert.equal(mergeConfirmMatch(" ct_2>ct_1 ", target, source), true);
  assert.equal(mergeConfirmMatch("", target, source), false);
  assert.equal(mergeConfirmMatch("other", target, source), false);
});

test("undoConfirmMatch, filter, page, labels", () => {
  assert.equal(undoConfirmMatch("ct_1", { id: "ct_1" }, "ct_2"), true);
  assert.equal(undoConfirmMatch("ct_2", { id: "ct_1" }, "ct_2"), true);
  assert.equal(undoConfirmMatch("nope", { id: "ct_1" }, "ct_2"), false);
  const a = row();
  const b = row({
    id: "ct_2",
    display: "discord:room",
    kind: "group",
    channel: "discord",
    dest: "room",
    identifiers: [{ channel: "discord", dest: "room", kind: "group", permission: "group" }],
  });
  assert.equal(filterContacts([a, b], "room", "", "").length, 1);
  assert.equal(filterContacts([a, b], "", "discord", "").length, 1);
  assert.equal(filterContacts([a, b], "", "", "user").length, 1);
  assert.equal(pageOf([a, b, a], 0, 2).length, 2);
  assert.equal(channelIdsLine(a), "telegram:111");
  assert.equal(lastSourceId({ merged_from: ["ct_9", "ct_8"] }), "ct_8");
});

test("mergePair uses detail as target and can swap direction", () => {
  const a = row({ id: "ct_keep", display: "Keep" });
  const b = row({ id: "ct_src", display: "Source", dest: "222" });
  assert.equal(mergePair(["ct_keep"], "ct_keep", [a, b]), null);
  const pair = mergePair(["ct_keep", "ct_src"], "ct_keep", [a, b]);
  assert.equal(pair?.target.id, "ct_keep");
  assert.equal(pair?.source.id, "ct_src");
  const swapped = swapMergePair(pair!);
  assert.equal(swapped.target.id, "ct_src");
  assert.equal(swapped.source.id, "ct_keep");
});
