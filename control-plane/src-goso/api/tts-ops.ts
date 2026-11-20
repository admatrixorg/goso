export const TTS_PROVIDERS = ["none", "openai", "elevenlabs", "google", "azure", "edge"] as const;
export type TTSProvider = (typeof TTS_PROVIDERS)[number];

export const TTS_APPLY = ["off", "reply", "all"] as const;
export type TTSApply = (typeof TTS_APPLY)[number];

export type TTSStatus = {
  provider: TTSProvider | string;
  enabled: boolean;
  configured: boolean;
  key_set: boolean;
  env_owned: boolean;
  source: string;
  voice?: string;
  model?: string;
  language?: string;
  region?: string;
  endpoint?: string;
  auto_apply: TTSApply | string;
  max_chars: number;
  timeout_ms: number;
};

export type TTSWrite = {
  provider: string;
  enabled: boolean;
  api_key?: string;
  voice: string;
  model: string;
  language: string;
  region: string;
  endpoint: string;
  auto_apply: string;
  max_chars: number;
  timeout_ms: number;
};

export type TTSTest = {
  ok: boolean;
  configured: boolean;
  provider: string;
  kind?: string;
  latency_ms: number;
  error?: string;
};

export type TTSTestView = {
  ok: boolean;
  configured: boolean;
  provider: string;
  kind: string;
  latency_ms: number;
  error: string;
};

const SECRET_KEYS = new Set([
  "token",
  "secret",
  "password",
  "hmac",
  "hmac_key",
  "bot_token",
  "access_token",
  "api_key",
  "authorization",
  "private_key",
  "xi-api-key",
  "xi_api_key",
  "subscription_key",
]);
const SECRET_VAL =
  /\b(sk-[A-Za-z0-9_-]{8,}|gsk_[A-Za-z0-9]+|xai-[A-Za-z0-9]+|AIza[A-Za-z0-9_-]+|Bearer\s+[A-Za-z0-9._\-+=/]{8,}|xi-[A-Za-z0-9]+|token=)/i;

export function publicHasSecrets(row: unknown): boolean {
  if (row == null || typeof row !== "object") return false;
  const rec = row as Record<string, unknown>;
  for (const [k, v] of Object.entries(rec)) {
    if (SECRET_KEYS.has(k.toLowerCase()) && typeof v === "string" && v.length > 0) return true;
    if (typeof v === "string" && SECRET_VAL.test(v)) return true;
    if (v && typeof v === "object" && publicHasSecrets(v)) return true;
    if (Array.isArray(v) && v.some((x) => publicHasSecrets(x))) return true;
  }
  return false;
}

export function emptyStatus(): TTSStatus {
  return {
    provider: "none",
    enabled: true,
    configured: false,
    key_set: false,
    env_owned: false,
    source: "none",
    auto_apply: "off",
    max_chars: 4096,
    timeout_ms: 15000,
  };
}

export function asPublicStatus(row: unknown): TTSStatus | undefined {
  if (!row || typeof row !== "object") return undefined;
  const rec = row as Record<string, unknown>;
  if (publicHasSecrets(rec)) return undefined;
  return {
    provider: String(rec.provider || "none"),
    enabled: rec.enabled !== false,
    configured: rec.configured === true,
    key_set: rec.key_set === true,
    env_owned: rec.env_owned === true,
    source: String(rec.source || "none"),
    voice: rec.voice ? String(rec.voice) : undefined,
    model: rec.model ? String(rec.model) : undefined,
    language: rec.language ? String(rec.language) : undefined,
    region: rec.region ? String(rec.region) : undefined,
    endpoint: rec.endpoint ? String(rec.endpoint) : undefined,
    auto_apply: String(rec.auto_apply || "off"),
    max_chars: Number(rec.max_chars) || 4096,
    timeout_ms: Number(rec.timeout_ms) || 15000,
  };
}

export function requiresKey(provider: string): boolean {
  const p = (provider || "").trim().toLowerCase();
  return p === "openai" || p === "elevenlabs" || p === "google" || p === "azure";
}

export function ttsWriteBody(form: TTSWrite): TTSWrite {
  const body: TTSWrite = {
    provider: (form.provider || "none").trim(),
    enabled: form.enabled !== false,
    voice: (form.voice || "").trim(),
    model: (form.model || "").trim(),
    language: (form.language || "").trim(),
    region: (form.region || "").trim(),
    endpoint: (form.endpoint || "").trim(),
    auto_apply: (form.auto_apply || "off").trim(),
    max_chars: Number(form.max_chars) || 4096,
    timeout_ms: Number(form.timeout_ms) || 15000,
  };
  const key = (form.api_key || "").trim();
  if (key) body.api_key = key;
  return body;
}

export function ttsConfirmMatch(typed: string, provider: string): boolean {
  const got = typed.trim().toLowerCase();
  if (!got) return false;
  if (got === "tts") return true;
  return got === (provider || "").trim().toLowerCase();
}

function redactTestText(s: string): string {
  let out = s
    .replace(/"(authorization|api[_-]?key|secret|token|password|xi-api-key|subscription_key)"\s*:\s*"(?:\\.|[^"\\])*"/gi, '"$1":"[redacted]"')
    .replace(/Bearer\s+[^\s"'\\]+/gi, "Bearer [redacted]")
    .replace(/\bsk-[A-Za-z0-9_*-]+\b/g, "sk-[redacted]");
  if (out.length > 400) out = `${out.slice(0, 400)}…`;
  return out;
}

export function formatTTSTest(raw: TTSTest | null | undefined): TTSTestView {
  const r = raw ?? { ok: false, configured: false, provider: "none", latency_ms: 0 };
  return {
    ok: r.ok === true,
    configured: r.configured === true,
    provider: String(r.provider || ""),
    kind: String(r.kind || ""),
    latency_ms: Number(r.latency_ms) || 0,
    error: r.error ? redactTestText(String(r.error)) : "",
  };
}

export function parseTTSTestError(e: unknown): TTSTestView | null {
  const s = String(e);
  const i = s.indexOf("{");
  if (i < 0) return null;
  try {
    const raw = JSON.parse(s.slice(i)) as TTSTest;
    if (typeof raw.ok !== "boolean") return null;
    return formatTTSTest(raw);
  } catch {
    return null;
  }
}

export function statusKind(row: TTSStatus): "disabled" | "not_configured" | "ready" {
  if (!row.enabled) return "disabled";
  if (!row.configured || row.provider === "none") return "not_configured";
  return "ready";
}
