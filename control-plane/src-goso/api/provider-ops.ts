export type ProviderInfo = {
  name: string;
  type: string;
  base_url: string;
  model: string;
  key_set: boolean;
  source: "env" | "sqlite" | string;
  enabled?: boolean;
};

export type ProviderTestResult = {
  ok: boolean;
  latency_ms: number;
  models?: string[];
  reply?: string;
  error?: string;
};

export type ProviderWrite = {
  name?: string;
  type?: string;
  base_url?: string;
  model?: string;
  api_key?: string;
  enabled?: boolean;
};

export type ProviderSourceFilter = "env" | "sqlite" | "";
export type ProviderEnabledFilter = "on" | "off" | "";

export type ProviderTestView = {
  ok: boolean;
  latency_ms: number;
  models: string[];
  reply: string;
  error: string;
};

export const PROVIDER_TYPES = ["openai-compat", "anthropic", "echo", "router9"] as const;

export function isProviderEnabled(p: { enabled?: boolean }): boolean {
  return p.enabled !== false;
}

export function isEnvOwned(p: { source?: string }): boolean {
  return (p.source || "") === "env";
}

export function canClearProviderKey(p: { source?: string; key_set?: boolean }): boolean {
  return p.source === "sqlite" && p.key_set === true;
}

export function filterProviders<
  T extends { name: string; type?: string; source?: string; model?: string; base_url?: string; enabled?: boolean },
>(rows: T[], opts: { query?: string; type?: string; source?: string; enabled?: ProviderEnabledFilter } = {}): T[] {
  const q = (opts.query || "").trim().toLowerCase();
  const typ = (opts.type || "").trim();
  const src = (opts.source || "").trim();
  const en = (opts.enabled || "").trim();
  return rows.filter((p) => {
    if (typ && (p.type || "") !== typ) return false;
    if (src && (p.source || "") !== src) return false;
    if (en === "on" && !isProviderEnabled(p)) return false;
    if (en === "off" && isProviderEnabled(p)) return false;
    if (!q) return true;
    const hay = `${p.name} ${p.type || ""} ${p.source || ""} ${p.model || ""} ${p.base_url || ""}`.toLowerCase();
    return hay.includes(q);
  });
}

export function uniqueProviderTypes(rows: { type?: string }[]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const t of PROVIDER_TYPES) {
    seen.add(t);
    out.push(t);
  }
  for (const r of rows) {
    const t = (r.type || "").trim();
    if (!t || seen.has(t)) continue;
    seen.add(t);
    out.push(t);
  }
  return out;
}

/** Build create/PATCH body. Blank api_key is omitted so empty PATCH cannot clear the boxed key. */
export function providerWriteBody(form: ProviderWrite): ProviderWrite {
  const body: ProviderWrite = {};
  const name = (form.name || "").trim();
  if (name) body.name = name;
  if (form.type != null) body.type = form.type;
  if (form.base_url != null) body.base_url = form.base_url;
  if (form.model != null) body.model = form.model;
  if (form.enabled != null) body.enabled = form.enabled;
  const key = (form.api_key || "").trim();
  if (key) body.api_key = key;
  return body;
}

function redactTestText(s: string): string {
  let out = s
    .replace(/"(authorization|api[_-]?key|secret|token|password)"\s*:\s*"(?:\\.|[^"\\])*"/gi, '"$1":"[redacted]"')
    .replace(/Bearer\s+[^\s"'\\]+/gi, "Bearer [redacted]")
    .replace(/\bsk-[A-Za-z0-9_*-]+\b/g, "sk-[redacted]");
  if (out.length > 400) out = `${out.slice(0, 400)}…`;
  return out;
}

/** Structured test view. Never stringify the raw payload (keys may hide in JSON). */
export function formatProviderTest(raw: ProviderTestResult | null | undefined): ProviderTestView {
  const r = raw ?? { ok: false, latency_ms: 0 };
  const models = Array.isArray(r.models)
    ? r.models.filter((m): m is string => typeof m === "string" && m.trim() !== "").slice(0, 50)
    : [];
  const reply = redactTestText(String(r.reply || "").trim());
  const error = r.error ? redactTestText(String(r.error)) : "";
  const ms = Number(r.latency_ms);
  return {
    ok: r.ok === true,
    latency_ms: Number.isFinite(ms) ? ms : 0,
    models,
    reply,
    error,
  };
}
