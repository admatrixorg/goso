import assert from "node:assert/strict";
import test from "node:test";
import {
  canClearBox,
  channelRemediation,
  filterChannels,
  formatAllowFrom,
  isPhase2,
  normalizeChannelRow,
  pairingExposesCode,
  parseAllowFrom,
  sanitizePairingItem,
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
