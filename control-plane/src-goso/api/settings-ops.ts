export type GatewayField = {
  key: string;
  value?: string | number | boolean;
  set?: boolean;
  env_owned?: boolean;
  editable?: boolean;
};

export type GatewayConfig = {
  updated_at?: string;
  server?: Record<string, GatewayField>;
  auth?: Record<string, GatewayField>;
  behavior?: Record<string, GatewayField>;
  quota?: Record<string, GatewayField>;
  tools?: Record<string, GatewayField>;
  integrations?: Record<string, GatewayField>;
};

export type GatewayPatch = { updated_at?: string; values: Record<string, string> };

export type GatewayForm = {
  log_level: string;
  quota_day: string;
  injection: string;
  ssrf: string;
  heartbeat: string;
  kg_extract: string;
  cache_mode: string;
};

export const GATEWAY_EDITABLE = ["log_level", "quota_day", "injection", "ssrf", "heartbeat", "kg_extract", "cache_mode"] as const;

export type GatewayEditable = (typeof GATEWAY_EDITABLE)[number];

const SECRET_RE = /(token|secret|password|api[_-]?key|master_key|database_url)/i;

export function emptyGatewayForm(): GatewayForm {
  return { log_level: "", quota_day: "", injection: "", ssrf: "", heartbeat: "", kg_extract: "", cache_mode: "" };
}

export function fieldValue(f?: GatewayField): string {
  if (!f || f.value == null) return "";
  if (typeof f.value === "boolean") return f.value ? "on" : "off";
  return String(f.value);
}

export function isFieldEnvOwned(f?: GatewayField): boolean {
  return f?.env_owned === true;
}

export function isFieldEditable(f?: GatewayField): boolean {
  return f?.editable === true && !isFieldEnvOwned(f);
}

function groupFields(cfg: GatewayConfig | null | undefined): GatewayField[] {
  if (!cfg) return [];
  const groups = [cfg.server, cfg.auth, cfg.behavior, cfg.quota, cfg.tools, cfg.integrations];
  const out: GatewayField[] = [];
  for (const g of groups) {
    if (!g) continue;
    for (const f of Object.values(g)) {
      if (f && typeof f === "object") out.push(f);
    }
  }
  return out;
}

export function publicHasSecrets(cfg: GatewayConfig | null | undefined): boolean {
  for (const f of groupFields(cfg)) {
    const key = String(f.key || "");
    if (SECRET_RE.test(key) && !/_set$/.test(key)) return true;
    const v = f.value;
    if (typeof v === "string" && /token|sk-|gsk_|xai-|AIza|postgres:\/\//i.test(v) && v.length > 8) return true;
  }
  return false;
}

export function formFromSnapshot(cfg: GatewayConfig | null | undefined): GatewayForm {
  const form = emptyGatewayForm();
  if (!cfg) return form;
  form.log_level = fieldValue(cfg.server?.log_level);
  form.quota_day = fieldValue(cfg.quota?.day_limit);
  form.injection = fieldValue(cfg.tools?.injection);
  form.ssrf = fieldValue(cfg.tools?.ssrf);
  form.heartbeat = fieldValue(cfg.behavior?.heartbeat);
  form.kg_extract = fieldValue(cfg.behavior?.kg_extract);
  form.cache_mode = fieldValue(cfg.behavior?.cache_mode);
  return form;
}

export function editableValues(form: GatewayForm, cfg: GatewayConfig | null | undefined): Record<string, string> {
  const out: Record<string, string> = {};
  const map: Record<GatewayEditable, GatewayField | undefined> = {
    log_level: cfg?.server?.log_level,
    quota_day: cfg?.quota?.day_limit,
    injection: cfg?.tools?.injection,
    ssrf: cfg?.tools?.ssrf,
    heartbeat: cfg?.behavior?.heartbeat,
    kg_extract: cfg?.behavior?.kg_extract,
    cache_mode: cfg?.behavior?.cache_mode,
  };
  for (const key of GATEWAY_EDITABLE) {
    if (!isFieldEditable(map[key])) continue;
    out[key] = form[key].trim();
  }
  return out;
}

export function validateGatewayForm(form: GatewayForm, values: Record<string, string>): string | null {
  if ("quota_day" in values) {
    const v = values.quota_day;
    if (v !== "" && (!/^\d+$/.test(v) || Number.parseInt(v, 10) < 0)) return "settings.invalidQuota";
  }
  if ("log_level" in values) {
    const v = form.log_level.trim().toLowerCase();
    if (v && !["debug", "info", "warn", "warning", "error"].includes(v)) return "settings.invalidLogLevel";
  }
  if ("injection" in values) {
    const v = form.injection.trim().toLowerCase();
    if (v && v !== "log" && v !== "block") return "settings.invalidInjection";
  }
  if ("cache_mode" in values) {
    const v = form.cache_mode.trim().toLowerCase();
    if (v && v !== "none" && v !== "full") return "settings.invalidCacheMode";
  }
  return null;
}

export function settingsConflictKind(err: unknown): "conflict" | "env_owned" | null {
  const s = String(err);
  if (!/\b409\b/.test(s)) return null;
  if (/env-owned/i.test(s)) return "env_owned";
  return "conflict";
}

export function boolLabel(v: unknown): "yes" | "no" {
  return v === true ? "yes" : "no";
}
