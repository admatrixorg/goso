import assert from "node:assert/strict";
import test from "node:test";
import {
  asPublicStatus,
  emptyStatus,
  formatTTSTest,
  parseTTSTestError,
  publicHasSecrets,
  requiresKey,
  statusKind,
  ttsConfirmMatch,
  ttsWriteBody,
} from "./tts-ops.ts";

test("asPublicStatus drops secret-shaped rows and keeps booleans", () => {
  const row = asPublicStatus({
    provider: "openai",
    enabled: true,
    configured: true,
    key_set: true,
    env_owned: false,
    source: "memory",
    voice: "alloy",
    auto_apply: "reply",
    max_chars: 512,
    timeout_ms: 8000,
  });
  assert.equal(row?.key_set, true);
  assert.equal(row?.configured, true);
  assert.equal("api_key" in (row || {}), false);
  assert.equal(asPublicStatus({ provider: "openai", api_key: "sk-liveSECRET99" }), undefined);
});

test("publicHasSecrets", () => {
  assert.equal(publicHasSecrets(emptyStatus()), false);
  assert.equal(publicHasSecrets({ provider: "openai", api_key: "x" }), true);
  assert.equal(publicHasSecrets({ error: "Bearer sk-abcdefghijk" }), true);
});

test("ttsWriteBody omits blank api_key", () => {
  const body = ttsWriteBody({
    provider: "openai",
    enabled: true,
    api_key: "  ",
    voice: "alloy",
    model: "tts-1",
    language: "",
    region: "",
    endpoint: "",
    auto_apply: "reply",
    max_chars: 512,
    timeout_ms: 8000,
  });
  assert.equal("api_key" in body, false);
  const withKey = ttsWriteBody({ ...body, api_key: "sk-new" });
  assert.equal(withKey.api_key, "sk-new");
});

test("formatTTSTest redacts authorization", () => {
  const view = formatTTSTest({
    ok: false,
    configured: true,
    provider: "openai",
    latency_ms: 12,
    error: `{"authorization":"Bearer sk-LEAKEDKEY99","api_key":"sk-LEAKEDKEY99"}`,
  });
  assert.equal(view.ok, false);
  assert.equal(view.error.includes("sk-LEAKED"), false);
  assert.match(view.error, /redacted/i);
  const parsed = parseTTSTestError('400 {"ok":false,"configured":true,"provider":"openai","latency_ms":3,"error":"unauthorized"}');
  assert.equal(parsed?.ok, false);
  assert.equal(parsed?.error, "unauthorized");
});

test("requiresKey statusKind confirm", () => {
  assert.equal(requiresKey("openai"), true);
  assert.equal(requiresKey("edge"), false);
  assert.equal(requiresKey("none"), false);
  assert.equal(statusKind(emptyStatus()), "not_configured");
  assert.equal(statusKind({ ...emptyStatus(), enabled: false, provider: "openai", configured: true }), "disabled");
  assert.equal(statusKind({ ...emptyStatus(), provider: "edge", configured: true }), "ready");
  assert.equal(ttsConfirmMatch("TTS", "openai"), true);
  assert.equal(ttsConfirmMatch("openai", "openai"), true);
  assert.equal(ttsConfirmMatch("", "openai"), false);
});
