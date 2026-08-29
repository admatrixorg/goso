export const SCOPES = ["admin", "read", "write", "approvals", "pairing", "provision"] as const;
export type ApiKeyScope = (typeof SCOPES)[number];

export type ApiKey = {
  id: string;
  name: string;
  prefix: string;
  tenant_id: string;
  scopes: string[];
  status: string;
  use_count: number;
  created_at?: string;
  expires_at?: string;
  last_used_at?: string;
  revoked_at?: string;
};

export type ApiKeyCreated = ApiKey & { secret?: string };

export type LastSecret = {
  id: string;
  name: string;
  prefix: string;
  secret?: string;
};

const SECRET_KEYS = new Set([
  "token",
  "secret",
  "password",
  "hmac",
  "hmac_key",
  "code",
  "bot_token",
  "access_token",
  "api_key",
  "hash",
  "key_hash",
  "authorization",
  "private_key",
]);
const SECRET_VAL =
  /\b(sk-[A-Za-z0-9_-]{8,}|gsk_[A-Za-z0-9]+|xai-[A-Za-z0-9]+|AIza[A-Za-z0-9_-]+|gk_[0-9a-f]{16,}|Bearer\s+[A-Za-z0-9._\-+=/]{8,}|token=)/i;

export function publicHasSecrets(row: unknown): boolean {
  if (row == null || typeof row !== "object") return false;
  const rec = row as Record<string, unknown>;
  for (const [k, v] of Object.entries(rec)) {
    if (SECRET_KEYS.has(k.toLowerCase()) && typeof v === "string" && v.length > 0) return true;
    if (typeof v === "string" && SECRET_VAL.test(v)) return true;
  }
  return false;
}

function scopesOf(row: { scopes?: unknown }): string[] {
  if (!Array.isArray(row.scopes)) return [];
  const out: string[] = [];
  for (const s of row.scopes) {
    const v = String(s || "").trim();
    if (v && !out.includes(v)) out.push(v);
  }
  return out;
}

export function asPublic(rows: ApiKey[] | null | undefined): ApiKey[] {
  const out: ApiKey[] = [];
  for (const row of rows || []) {
    if (!row || publicHasSecrets(row)) continue;
    const id = String(row.id || "").trim();
    if (!id) continue;
    out.push({
      id,
      name: String(row.name || ""),
      prefix: String(row.prefix || ""),
      tenant_id: String(row.tenant_id || ""),
      scopes: scopesOf(row),
      status: String(row.status || ""),
      use_count: Number.isFinite(Number(row.use_count)) ? Number(row.use_count) : 0,
      created_at: row.created_at ? String(row.created_at) : undefined,
      expires_at: row.expires_at ? String(row.expires_at) : undefined,
      last_used_at: row.last_used_at ? String(row.last_used_at) : undefined,
      revoked_at: row.revoked_at ? String(row.revoked_at) : undefined,
    });
  }
  return out;
}

export function asCreated(row: ApiKeyCreated | null | undefined): LastSecret | null {
  if (!row) return null;
  const id = String(row.id || "").trim();
  const secret = typeof row.secret === "string" && row.secret.trim() ? row.secret : undefined;
  if (!id || !secret) return null;
  return {
    id,
    name: String(row.name || ""),
    prefix: String(row.prefix || ""),
    secret,
  };
}

export function hideCopiedSecret(last: LastSecret): LastSecret {
  return { ...last, secret: undefined };
}

export function filterKeys(rows: ApiKey[], q: string): ApiKey[] {
  const needle = (q || "").trim().toLowerCase();
  if (!needle) return rows;
  return rows.filter((r) => {
    const hay = `${r.id} ${r.name} ${r.prefix} ${r.tenant_id} ${r.status} ${r.scopes.join(" ")}`.toLowerCase();
    return hay.includes(needle);
  });
}

export function keyConfirmMatch(typed: string, row: Pick<ApiKey, "id" | "name" | "prefix">): boolean {
  const v = (typed || "").trim();
  if (!v) return false;
  return v === row.id || v === row.name || v === row.prefix;
}

export function keyLabel(row: Pick<ApiKey, "id" | "name" | "prefix">): string {
  return (row.name || "").trim() || row.prefix || row.id;
}

export function maskedPrefix(prefix: string): string {
  const p = (prefix || "").trim();
  if (!p) return "";
  return `${p}…`;
}

export function formatWhen(iso: string | undefined, fallback: string): string {
  const s = (iso || "").trim();
  if (!s) return fallback;
  const d = new Date(s);
  if (Number.isNaN(d.getTime())) return s;
  return d.toLocaleString();
}

export function usageLabel(row: Pick<ApiKey, "use_count" | "last_used_at">, never: string): string {
  const n = row.use_count || 0;
  if (n <= 0) return never;
  const last = formatWhen(row.last_used_at, "");
  return last ? `${n} · ${last}` : String(n);
}

export function isKnownScope(s: string): s is ApiKeyScope {
  return (SCOPES as readonly string[]).includes(s);
}

export function toggleScope(selected: string[], scope: string): string[] {
  const s = scope.trim();
  if (!s) return selected;
  if (selected.includes(s)) return selected.filter((x) => x !== s);
  return [...selected, s];
}
