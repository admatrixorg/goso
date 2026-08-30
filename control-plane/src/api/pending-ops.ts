import type { PendingGroup } from "./pending";

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

export function asPublic(groups: PendingGroup[] | null | undefined): PendingGroup[] {
  const out: PendingGroup[] = [];
  for (const g of groups || []) {
    if (!g || publicHasSecrets(g)) continue;
    out.push({
      id: String(g.id || ""),
      channel: String(g.channel || ""),
      dest: String(g.dest || ""),
      agent_id: g.agent_id ? String(g.agent_id) : undefined,
      agent: g.agent ? String(g.agent) : undefined,
      count: Number(g.count) || 0,
      oldest_at: g.oldest_at,
      newest_at: g.newest_at,
      age_ms: typeof g.age_ms === "number" ? g.age_ms : undefined,
      compacted: Boolean(g.compacted),
      compacting: Boolean(g.compacting),
      compacted_from: g.compacted_from,
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

export function pendingConfirmMatch(typed: string, group: Pick<PendingGroup, "id" | "dest" | "channel">): boolean {
  const v = (typed || "").trim();
  if (!v) return false;
  if (v === group.id || v === group.dest) return true;
  return v === `${group.channel}:${group.dest}`;
}

export function groupLabel(group: Pick<PendingGroup, "channel" | "dest">): string {
  const ch = (group.channel || "").trim();
  const dest = (group.dest || "").trim();
  if (ch && dest) return `${ch}:${dest}`;
  return dest || ch || "";
}

export function agentLabel(group: Pick<PendingGroup, "agent" | "agent_id">, na: string): string {
  const name = (group.agent || "").trim();
  if (name) return name;
  const id = (group.agent_id || "").trim();
  if (id) return id;
  return na;
}

export function formatAge(ageMs: number | undefined): string {
  const ms = typeof ageMs === "number" && ageMs >= 0 ? ageMs : 0;
  const sec = Math.floor(ms / 1000);
  if (sec < 60) return `${sec}s`;
  const min = Math.floor(sec / 60);
  if (min < 60) return `${min}m`;
  const hr = Math.floor(min / 60);
  if (hr < 24) return `${hr}h`;
  const day = Math.floor(hr / 24);
  return `${day}d`;
}

export function previewLine(group: PendingGroup, na: string): string {
  return [groupLabel(group), agentLabel(group, na), String(group.count ?? 0), formatAge(group.age_ms)].join(" · ");
}

export type PendingWriteKind = "busy" | "permission" | "mismatch" | "missing" | "error";

/** Compact/clear failures stay on the action, not the inventory empty/permission claim. */
export function pendingWriteKind(err: unknown): PendingWriteKind {
  const s = String(err);
  if (/\b409\b/.test(s) || /compact in progress/i.test(s)) return "busy";
  if (/\b401\b/.test(s) || /\b403\b/.test(s)) return "permission";
  if (/confirm does not match/i.test(s) || /mismatch/i.test(s)) return "mismatch";
  if (/\b404\b/.test(s)) return "missing";
  return "error";
}

export function compactBlocked(group: Pick<PendingGroup, "compacting">, busy: boolean): boolean {
  return Boolean(group.compacting) || busy;
}
