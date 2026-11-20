import type { Contact, ContactIdent } from "./contacts";

export const PAGE_SIZE = 50;

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

export function asPublic(rows: Contact[] | null | undefined): Contact[] {
  const out: Contact[] = [];
  for (const row of rows || []) {
    if (!row || publicHasSecrets(row)) continue;
    const identifiers = (row.identifiers || []).filter((id) => id && !publicHasSecrets(id)).map(asPublicIdent);
    out.push({
      id: String(row.id || ""),
      display: String(row.display || ""),
      kind: String(row.kind || ""),
      channel: String(row.channel || ""),
      dest: String(row.dest || ""),
      identifiers,
      count: Number(row.count) || 0,
      first_seen: row.first_seen,
      last_seen: row.last_seen,
      permission: row.permission ? String(row.permission) : undefined,
      agent_id: row.agent_id ? String(row.agent_id) : undefined,
      agent: row.agent ? String(row.agent) : undefined,
      can_undo: Boolean(row.can_undo),
      merged_from: Array.isArray(row.merged_from) ? row.merged_from.map(String) : undefined,
    });
  }
  return out;
}

function asPublicIdent(id: ContactIdent): ContactIdent {
  return {
    channel: String(id.channel || ""),
    dest: String(id.dest || ""),
    kind: String(id.kind || ""),
    permission: String(id.permission || ""),
  };
}

export function publicHasSecrets(row: unknown): boolean {
  if (row == null || typeof row !== "object") return false;
  const rec = row as Record<string, unknown>;
  for (const [k, v] of Object.entries(rec)) {
    if (k === "identifiers" && Array.isArray(v)) {
      if (v.some((item) => publicHasSecrets(item))) return true;
      continue;
    }
    if (SECRET_KEYS.has(k.toLowerCase()) && typeof v === "string" && v.length > 0) return true;
    if (typeof v === "string" && SECRET_VAL.test(v)) return true;
  }
  return false;
}

export function mergeConfirmMatch(typed: string, target: Pick<Contact, "id">, source: Pick<Contact, "id" | "dest">): boolean {
  const v = (typed || "").trim();
  if (!v) return false;
  if (v === source.id || v === target.id || v === source.dest) return true;
  return v === `${source.id}>${target.id}`;
}

export function undoConfirmMatch(typed: string, target: Pick<Contact, "id">, lastSource: string): boolean {
  const v = (typed || "").trim();
  if (!v) return false;
  return v === target.id || (lastSource !== "" && v === lastSource);
}

export function identLabel(id: Pick<ContactIdent, "channel" | "dest">): string {
  const ch = (id.channel || "").trim();
  const dest = (id.dest || "").trim();
  if (ch && dest) return `${ch}:${dest}`;
  return dest || ch || "";
}

export function channelIdsLine(row: Pick<Contact, "identifiers" | "channel" | "dest">): string {
  const ids = row.identifiers || [];
  if (ids.length) return ids.map(identLabel).join(", ");
  return identLabel({ channel: row.channel, dest: row.dest });
}

export function uniqueChannels(rows: Contact[]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const row of rows) {
    for (const id of row.identifiers || []) {
      const ch = (id.channel || "").trim();
      if (!ch || seen.has(ch)) continue;
      seen.add(ch);
      out.push(ch);
    }
    const ch = (row.channel || "").trim();
    if (ch && !seen.has(ch)) {
      seen.add(ch);
      out.push(ch);
    }
  }
  return out;
}

export function filterContacts(rows: Contact[], q: string, channel: string, kind: string): Contact[] {
  const needle = (q || "").trim().toLowerCase();
  const ch = (channel || "").trim();
  const k = (kind || "").trim().toLowerCase();
  return rows.filter((row) => {
    if (k && (row.kind || "").toLowerCase() !== k) return false;
    if (ch) {
      const hit = (row.channel || "") === ch || (row.identifiers || []).some((id) => id.channel === ch);
      if (!hit) return false;
    }
    if (!needle) return true;
    const blob = [row.id, row.display, row.channel, row.dest, channelIdsLine(row)].join(" ").toLowerCase();
    return blob.includes(needle);
  });
}

export function pageOf<T>(rows: T[], offset: number, size = PAGE_SIZE): T[] {
  const start = Math.max(0, offset);
  return rows.slice(start, start + size);
}

export function lastSourceId(row: Pick<Contact, "merged_from">): string {
  const from = row.merged_from || [];
  return from.length ? String(from[from.length - 1]) : "";
}

export function mergePair(
  selected: string[],
  detailId: string,
  rows: Contact[],
): { target: Contact; source: Contact } | null {
  if (selected.length !== 2) return null;
  const [a, b] = selected;
  const targetId = selected.includes(detailId) ? detailId : a;
  const sourceId = targetId === a ? b : a;
  const target = rows.find((row) => row.id === targetId);
  const source = rows.find((row) => row.id === sourceId);
  if (!target || !source || target.id === source.id) return null;
  return { target, source };
}

export function swapMergePair(pair: { target: Contact; source: Contact }): { target: Contact; source: Contact } {
  return { target: pair.source, source: pair.target };
}
