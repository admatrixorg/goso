import type { NodeDevice } from "./nodes";

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
]);
const SECRET_VAL = /\b(sk-[A-Za-z0-9_-]{8,}|gsk_[A-Za-z0-9]+|xai-[A-Za-z0-9]+|Bearer\s+[A-Za-z0-9._\-+=/]{8,})/i;

export function asPublic(rows: NodeDevice[] | null | undefined): NodeDevice[] {
  const out: NodeDevice[] = [];
  for (const row of rows || []) {
    if (!row || publicHasSecrets(row)) continue;
    out.push({
      id: String(row.id || ""),
      display: String(row.display || ""),
      kind: String(row.kind || ""),
      status: String(row.status || ""),
      health: String(row.health || ""),
      requested_at: row.requested_at ? String(row.requested_at) : undefined,
      expires_at: row.expires_at ? String(row.expires_at) : undefined,
      approved_at: row.approved_at ? String(row.approved_at) : undefined,
      last_seen: row.last_seen ? String(row.last_seen) : undefined,
    });
  }
  return out;
}

export function publicHasSecrets(row: unknown): boolean {
  if (row == null || typeof row !== "object") return false;
  const rec = row as Record<string, unknown>;
  for (const [k, v] of Object.entries(rec)) {
    if (SECRET_KEYS.has(k.toLowerCase()) && typeof v === "string" && v.length > 0) return true;
    if (typeof v === "string" && SECRET_VAL.test(v)) return true;
  }
  return false;
}

export function nodeConfirmMatch(typed: string, row: Pick<NodeDevice, "id" | "display">): boolean {
  const v = (typed || "").trim();
  if (!v) return false;
  return v === row.id || v === row.display;
}

export function nodeLabel(row: Pick<NodeDevice, "id" | "display">): string {
  return (row.display || "").trim() || row.id;
}

/** Combined inventory size for page-state. Pending-empty and paired-empty are section-local after a successful load. */
export function nodeInventoryCount(pending: NodeDevice[] | null | undefined, paired: NodeDevice[] | null | undefined): number {
  return (pending || []).length + (paired || []).length;
}

export function formatWhen(iso: string | undefined, fallback: string): string {
  const s = (iso || "").trim();
  if (!s) return fallback;
  const d = new Date(s);
  if (Number.isNaN(d.getTime())) return s;
  return d.toLocaleString();
}
