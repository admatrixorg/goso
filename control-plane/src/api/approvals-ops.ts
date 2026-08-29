export type Approval = {
  id: string;
  approval_id: string;
  kind: string;
  requester: string;
  agent_id: string;
  session_id?: string;
  connector: string;
  tool: string;
  arg_preview: string;
  risk: string;
  status: string;
  expires_at?: string;
  created_at?: string;
  decided_at?: string;
  decision?: string;
  reason?: string;
  stale: boolean;
  relay_error?: string;
};

export type ApprovalList = {
  approvals: Approval[];
  pending: number;
  generated_at?: string;
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
  "authorization",
  "private_key",
  "args",
  "arguments",
  "content",
  "body",
]);
const SECRET_VAL =
  /\b(sk-[A-Za-z0-9_-]{8,}|gsk_[A-Za-z0-9]+|xai-[A-Za-z0-9]+|AIza[A-Za-z0-9_-]+|gk_[0-9a-f]{16,}|Bearer\s+[A-Za-z0-9._\-+=/]{8,}|ghp_[A-Za-z0-9]+|token=)/i;

export function publicHasSecrets(row: unknown): boolean {
  if (row == null || typeof row !== "object") return false;
  const rec = row as Record<string, unknown>;
  for (const [k, v] of Object.entries(rec)) {
    const lk = k.toLowerCase();
    if (SECRET_KEYS.has(lk) && typeof v === "string" && v.length > 0) return true;
    if (lk === "args" || lk === "arguments") return true;
    if (typeof v === "string" && SECRET_VAL.test(v)) return true;
  }
  return false;
}

function str(v: unknown): string {
  return v == null ? "" : String(v);
}

export function asPublic(rows: Approval[] | null | undefined): Approval[] {
  const out: Approval[] = [];
  for (const row of rows || []) {
    if (!row || publicHasSecrets(row)) continue;
    const id = str(row.id || row.approval_id).trim();
    if (!id) continue;
    out.push({
      id,
      approval_id: str(row.approval_id || id),
      kind: str(row.kind) || "execution",
      requester: str(row.requester),
      agent_id: str(row.agent_id),
      session_id: row.session_id ? str(row.session_id) : undefined,
      connector: str(row.connector),
      tool: str(row.tool),
      arg_preview: str(row.arg_preview) || "{}",
      risk: str(row.risk) || "medium",
      status: str(row.status),
      expires_at: row.expires_at ? str(row.expires_at) : undefined,
      created_at: row.created_at ? str(row.created_at) : undefined,
      decided_at: row.decided_at ? str(row.decided_at) : undefined,
      decision: row.decision ? str(row.decision) : undefined,
      reason: row.reason ? str(row.reason) : undefined,
      stale: Boolean(row.stale) || isExpired(row),
      relay_error: row.relay_error ? str(row.relay_error) : undefined,
    });
  }
  return out;
}

export function asPublicList(j: Partial<ApprovalList> | null | undefined): ApprovalList {
  const approvals = asPublic(j?.approvals);
  return {
    approvals,
    pending: approvals.filter((r) => r.status === "pending").length,
    generated_at: j?.generated_at ? str(j.generated_at) : undefined,
  };
}

export function listHasSecrets(j: Partial<ApprovalList> | null | undefined): boolean {
  if (publicHasSecrets(j)) return true;
  return (j?.approvals || []).some((row) => publicHasSecrets(row));
}

export function isExpired(row: Pick<Approval, "status" | "expires_at">, now = Date.now()): boolean {
  if (row.status === "expired") return true;
  const s = (row.expires_at || "").trim();
  if (!s) return false;
  const t = Date.parse(s);
  return Number.isFinite(t) && t <= now;
}

export function canResolve(row: Approval, now = Date.now()): boolean {
  return row.status === "pending" && !isExpired(row, now);
}

export function approvalLabel(row: Pick<Approval, "id" | "tool" | "connector">): string {
  const tool = (row.tool || "").trim();
  const conn = (row.connector || "").trim();
  if (tool && conn) return `${conn}/${tool}`;
  return tool || conn || row.id;
}

export function formatWhen(iso: string | undefined, fallback: string): string {
  const s = (iso || "").trim();
  if (!s) return fallback;
  const d = new Date(s);
  if (Number.isNaN(d.getTime())) return s;
  return d.toLocaleString();
}

export const POLL_MS = 3000;
export const STALE_MS = 10000;
