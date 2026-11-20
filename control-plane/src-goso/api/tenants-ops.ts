export type TenantMember = {
  id: string;
  subject: string;
  role: string;
  created_at?: string;
};

export type Tenant = {
  id: string;
  name: string;
  status: string;
  master: boolean;
  created_at?: string;
  members?: TenantMember[];
};

export type TenantContext = {
  id: string;
  name: string;
  status?: string;
  master?: boolean;
};

export type TenantList = {
  tenants: Tenant[];
  current?: TenantContext;
  master?: TenantContext;
  multi_tenant?: boolean;
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
  "content",
  "text",
  "body",
  "api_key",
  "private_key",
  "authorization",
]);
const SECRET_VAL =
  /\b(sk-[A-Za-z0-9_-]{8,}|gsk_[A-Za-z0-9]+|xai-[A-Za-z0-9]+|AIza[A-Za-z0-9_-]+|Bearer\s+[A-Za-z0-9._\-+=/]{8,}|token=)/i;

export const ROLES = ["owner", "admin", "member", "viewer"] as const;

export function publicHasSecrets(row: unknown): boolean {
  if (row == null || typeof row !== "object") return false;
  const rec = row as Record<string, unknown>;
  for (const [k, v] of Object.entries(rec)) {
    if (SECRET_KEYS.has(k.toLowerCase()) && typeof v === "string" && v.length > 0) return true;
    if (typeof v === "string" && SECRET_VAL.test(v)) return true;
    if (Array.isArray(v) && v.some((item) => publicHasSecrets(item))) return true;
  }
  return false;
}

export function asPublicMember(row: TenantMember | null | undefined): TenantMember | null {
  if (!row || publicHasSecrets(row)) return null;
  const id = String(row.id || "").trim();
  const subject = String(row.subject || "").trim();
  if (!id || !subject) return null;
  return {
    id,
    subject,
    role: String(row.role || "member"),
    created_at: row.created_at ? String(row.created_at) : undefined,
  };
}

export function asPublicContext(row: TenantContext | null | undefined): TenantContext | undefined {
  if (!row || publicHasSecrets(row)) return undefined;
  const id = String(row.id || "").trim();
  if (!id) return undefined;
  return {
    id,
    name: String(row.name || id),
    status: row.status ? String(row.status) : undefined,
    master: Boolean(row.master),
  };
}

export function asPublic(rows: Tenant[] | null | undefined): Tenant[] {
  const out: Tenant[] = [];
  for (const row of rows || []) {
    if (!row || publicHasSecrets(row)) continue;
    const members = (row.members || []).map(asPublicMember).filter((m): m is TenantMember => Boolean(m));
    out.push({
      id: String(row.id || ""),
      name: String(row.name || ""),
      status: String(row.status || ""),
      master: Boolean(row.master),
      created_at: row.created_at ? String(row.created_at) : undefined,
      members,
    });
  }
  return out.filter((r) => r.id);
}

export function filterTenants(rows: Tenant[], q: string): Tenant[] {
  const needle = (q || "").trim().toLowerCase();
  if (!needle) return rows;
  return rows.filter((r) => {
    const hay = `${r.id} ${r.name} ${r.status}`.toLowerCase();
    return hay.includes(needle);
  });
}

export function tenantConfirmMatch(typed: string, row: Pick<Tenant, "id" | "name">): boolean {
  const v = (typed || "").trim();
  if (!v) return false;
  return v === row.id || v === row.name;
}

export function memberConfirmMatch(typed: string, row: Pick<TenantMember, "id" | "subject">): boolean {
  const v = (typed || "").trim();
  if (!v) return false;
  return v === row.id || v === row.subject;
}

export function tenantLabel(row: Pick<Tenant, "id" | "name">): string {
  return (row.name || "").trim() || row.id;
}

export function formatWhen(iso: string | undefined, fallback: string): string {
  const s = (iso || "").trim();
  if (!s) return fallback;
  const d = new Date(s);
  if (Number.isNaN(d.getTime())) return s;
  return d.toLocaleString();
}
