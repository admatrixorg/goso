import assert from "node:assert/strict";
import test from "node:test";
import { classifyPageState, isFilteredEmpty } from "./page-state.ts";
import {
  canClearBox,
  channelRemediation,
  emptySecretDraft,
  filterChannels,
  formatAllowFrom,
  isPhase2,
  isSecretDraftEmpty,
  normalizeChannelRow,
  pairingConfirmMatch,
  pairingExposesCode,
  pairingLabel,
  pairingListHasSecrets,
  parseAllowFrom,
  publicPairingList,
  resolveSettled,
  sanitizePairingItem,
  secretMetaKind,
  secretPutBody,
} from "./channel-ops.ts";

const rows = [
  { name: "telegram", health: "running" },
  { name: "zalo-oa", health: "missing" },
  { name: "discord", health: "parked" },
];

test("filterChannels matches name and health", () => {
  assert.equal(filterChannels(rows, { query: "tele" }).map((c) => c.name).join(), "telegram");
  assert.equal(filterChannels(rows, { query: "OA" }).map((c) => c.name).join(), "zalo-oa");
  assert.equal(filterChannels(rows, { health: "parked" }).map((c) => c.name).join(), "discord");
  assert.equal(filterChannels(rows, { query: "  ", health: "" }).length, 3);
});

test("filtered empty is distinct from catalog error and true empty", () => {
  const ready = classifyPageState({ loading: false, loaded: true, error: null, itemCount: 3 });
  const visible = filterChannels(rows, { query: "nope" });
  assert.equal(isFilteredEmpty(ready, rows.length, visible.length), true);
  const empty = classifyPageState({ loading: false, loaded: true, error: null, itemCount: 0 });
  assert.equal(isFilteredEmpty(empty, 0, 0), false);
  const boom = classifyPageState({ loading: false, loaded: false, error: new Error("non-JSON response"), itemCount: 0 });
  assert.equal(boom.showEmpty, false);
  assert.equal(isFilteredEmpty(boom, 0, 0), false);
});

test("secretPutBody skips blanks and unknown fields", () => {
  assert.deepEqual(secretPutBody(["bot_token"], { bot_token: "  abc  ", extra: "nope" }), { bot_token: "abc" });
  assert.deepEqual(secretPutBody(["bot_token"], { bot_token: "   " }), {});
  assert.deepEqual(secretPutBody(["access_token", "app_secret"], { access_token: "a", app_secret: "" }), {
    access_token: "a",
  });
});

test("parseAllowFrom splits and dedupes", () => {
  assert.deepEqual(parseAllowFrom("u1, u2\nu1  u3"), ["u1", "u2", "u3"]);
  assert.deepEqual(parseAllowFrom("  "), []);
  assert.equal(formatAllowFrom(["a", "b"]), "a\nb");
});

test("channelRemediation prefers parked then failed then missing", () => {
  assert.equal(channelRemediation({ phase: 2, health: "parked" }), "parked");
  assert.equal(channelRemediation({ health: "failed", last_error: "redacted" }), "failed");
  assert.equal(channelRemediation({ health: "running", last_error: "boom" }), "failed");
  assert.equal(channelRemediation({ health: "missing", missing: true }), "missing");
  assert.equal(channelRemediation({ health: "running", from_env: true }), "from_env");
  assert.equal(channelRemediation({ health: "stopped" }), "stopped");
  assert.equal(channelRemediation({ health: "running" }), "ok");
});

test("canClearBox needs writable and secret_set", () => {
  assert.equal(canClearBox({ writable: ["bot_token"], secret_set: true }), true);
  assert.equal(canClearBox({ writable: ["bot_token"], secret_set: false }), false);
  assert.equal(canClearBox({ writable: [], secret_set: true }), false);
  assert.equal(isPhase2({ phase: 2 }), true);
  assert.equal(isPhase2({ health: "parked" }), true);
  assert.equal(isPhase2({ phase: 1, health: "running" }), false);
});

test("normalizeChannelRow drops blank names and keeps flags", () => {
  assert.equal(normalizeChannelRow({ name: "" }), null);
  const row = normalizeChannelRow({
    name: "telegram",
    configured: true,
    secret_set: true,
    writable: ["bot_token"],
    health: "running",
    dm_policy: "pairing",
    last_error: "redacted",
  });
  assert.equal(row?.name, "telegram");
  assert.equal(row?.secret_set, true);
  assert.equal(row?.missing, false);
  assert.deepEqual(row?.writable, ["bot_token"]);
  assert.equal(row?.last_error, "redacted");
});

test("sanitizePairingItem never keeps code fields", () => {
  const raw = {
    id: "p1",
    channel: "telegram",
    sender_id: "777",
    status: "pending",
    expires_at: "2026-08-30T00:00:00Z",
    code: "ABCD1234",
    code_hash: "deadbeef",
  };
  assert.equal(pairingExposesCode(raw), true);
  const clean = sanitizePairingItem(raw);
  assert.deepEqual(clean, {
    id: "p1",
    channel: "telegram",
    sender_id: "777",
    status: "pending",
    expires_at: "2026-08-30T00:00:00Z",
  });
  assert.equal(pairingExposesCode(clean), false);
  assert.equal("code" in clean, false);
  assert.equal("code_hash" in clean, false);
});

test("publicPairingList drops code-bearing and non-pending rows", () => {
  const items = publicPairingList([
    { id: "p1", channel: "telegram", sender_id: "777", status: "pending" },
    { id: "p2", channel: "telegram", sender_id: "1", status: "pending", code: "ABCD1234" },
    { id: "p3", channel: "zalo-oa", sender_id: "2", status: "approved" },
    { id: "", channel: "telegram", sender_id: "3", status: "pending" },
  ]);
  assert.equal(items.length, 1);
  assert.equal(items[0].id, "p1");
  assert.equal("code" in items[0], false);
  assert.equal(pairingListHasSecrets([{ id: "p2", code: "ABCD1234" }]), true);
  assert.equal(pairingListHasSecrets(items), false);
});

test("pairingConfirmMatch uses id or sender, never a code", () => {
  const item = { id: "p1", sender_id: "777" };
  assert.equal(pairingConfirmMatch("p1", item), true);
  assert.equal(pairingConfirmMatch("777", item), true);
  assert.equal(pairingConfirmMatch(" ABCD1234 ", item), false);
  assert.equal(pairingConfirmMatch("", item), false);
  assert.equal(pairingLabel({ id: "p1", channel: "telegram", sender_id: "777" }), "telegram · 777");
});

test("empty secret draft never hydrates and empty save body is rejected", () => {
  const draft = emptySecretDraft();
  assert.equal(draft.bot_token, "");
  assert.equal(draft.access_token, "");
  assert.equal(draft.app_secret, "");
  assert.equal(isSecretDraftEmpty(["bot_token"], draft), true);
  assert.deepEqual(secretPutBody(["bot_token"], draft), {});
  assert.equal(secretMetaKind({ secret_set: true, from_env: true }), "env");
  assert.equal(secretMetaKind({ secret_set: true, from_env: false }), "set");
  assert.equal(secretMetaKind({ secret_set: false, from_env: false }), "unset");
});

test("resolveSettled keeps catalog and pairing failures independent", () => {
  const failed: PromiseSettledResult<number> = { status: "rejected", reason: new Error("401 unauthorized") };
  const ok: PromiseSettledResult<number> = { status: "fulfilled", value: 7 };
  const a = resolveSettled(failed);
  const b = resolveSettled(ok);
  assert.equal(a.ok, false);
  if (!a.ok) assert.match(String(a.error), /401/);
  assert.equal(b.ok, true);
  if (b.ok) assert.equal(b.value, 7);
  const catalog = classifyPageState({ loading: false, loaded: false, error: new Error("non-JSON response"), itemCount: 0 });
  const pairing = classifyPageState({ loading: false, loaded: true, error: null, itemCount: 1 });
  assert.equal(catalog.kind, "error");
  assert.equal(catalog.showEmpty, false);
  assert.equal(pairing.kind, "ready");
  assert.equal(pairing.showEmpty, false);
});
