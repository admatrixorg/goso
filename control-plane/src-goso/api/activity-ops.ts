import {
  classifyPageState,
  inventoryBlocksMutation,
  type PageLoadKind,
  type PageState,
} from "./page-state.ts";

export const PAGE_SIZE = 25;
export const META_CAP = 200;

export type ActivityRecord = {
  seq: number;
  id: string;
  action: string;
  actor: string;
  entity: string;
  entity_id?: string;
  ip?: string;
  ts: string;
  before?: Record<string, unknown>;
  after?: Record<string, unknown>;
};

export type ActivityPage = {
  records: ActivityRecord[];
  total: number;
  limit: number;
  before?: number;
  next_before?: number;
};

export type ActivityQuery = {
  action?: string;
  actor?: string;
  entity?: string;
  ip?: string;
  since?: string;
  until?: string;
  limit?: number;
  before?: number;
};

export type ActivityDetail = { key: string; value: string };

const SECRET_KEYS = [
  "token",
  "password",
  "secret",
  "authorization",
  "api_key",
  "apikey",
  "bearer",
  "credential",
  "hmac",
  "private_key",
  "bot_token",
  "access_token",
  "hmac_key",
];

const PAYLOAD_KEYS = [
  "arguments",
  "args",
  "body",
  "content",
  "messages",
  "prompt",
  "result",
  "tool_input",
  "tool_result",
  "text",
  "input",
  "output",
  "message",
];

const META_KEYS = [
  "ok",
  "enabled",
  "status",
  "path",
  "size",
  "fields",
  "key_set",
  "key_rotated",
  "type",
  "backend",
  "host",
  "health",
  "count",
  "source_id",
  "secret_set",
  "agent_key",
  "truncated",
  "bytes",
];

const TOKEN_SHAPE = /\b(sk-[A-Za-z0-9_-]{8,}|Bearer\s+[A-Za-z0-9._\-+=/]{8,})/i;

function hideKey(k: string): boolean {
  const lk = k.toLowerCase();
  if (lk.endsWith("_set")) return false;
  return SECRET_KEYS.some((sk) => lk === sk || lk.includes(sk)) || PAYLOAD_KEYS.some((pk) => lk === pk);
}

function capText(s: string, n: number): string {
  const v = (s || "").replace(/\u0000/g, "");
  if (v.length <= n) return v;
  return `${v.slice(0, n)}…`;
}

function publicValue(v: unknown): unknown {
  if (v == null) return v;
  if (typeof v === "string") return TOKEN_SHAPE.test(v) ? "[redacted]" : capText(v, META_CAP);
  if (typeof v === "number" || typeof v === "boolean") return v;
  if (Array.isArray(v)) return v.map(publicValue).slice(0, 16);
  if (typeof v === "object") return publicMeta(v as Record<string, unknown>);
  return String(v);
}

export function publicMeta(raw: Record<string, unknown> | undefined): Record<string, unknown> | undefined {
  if (!raw || typeof raw !== "object") return undefined;
  const out: Record<string, unknown> = {};
  let n = 0;
  for (const [k, v] of Object.entries(raw)) {
    if (n >= 16) break;
    if (!k || hideKey(k)) continue;
    out[k] = publicValue(v);
    n++;
  }
  return Object.keys(out).length ? out : undefined;
}

export function asPublicRecord(raw: ActivityRecord | null | undefined): ActivityRecord | null {
  if (!raw || typeof raw !== "object") return null;
  const before = publicMeta(raw.before);
  const after = publicMeta(raw.after);
  const row: ActivityRecord = {
    seq: typeof raw.seq === "number" ? raw.seq : 0,
    id: String(raw.id || ""),
    action: String(raw.action || ""),
    actor: String(raw.actor || ""),
    entity: String(raw.entity || ""),
    entity_id: raw.entity_id ? String(raw.entity_id) : undefined,
    ip: raw.ip ? String(raw.ip) : undefined,
    ts: String(raw.ts || ""),
    before,
    after,
  };
  if (publicHasSecrets(row)) return null;
  return row;
}

export function publicHasSecrets(row: unknown): boolean {
  const s = JSON.stringify(row ?? "");
  if (TOKEN_SHAPE.test(s)) return true;
  if (/"token"\s*:\s*"[^"]+"/i.test(s) && !/"token"\s*:\s*"\[redacted\]"/i.test(s)) return true;
  if (/"api_key"\s*:\s*"[^"]+"/i.test(s)) return true;
  if (/"body"\s*:/.test(s) || /"arguments"\s*:/.test(s)) return true;
  return false;
}

export function parseDetail(row: ActivityRecord): ActivityDetail[] {
  const out: ActivityDetail[] = [];
  const push = (key: string, value: unknown) => {
    if (value == null || value === "") return;
    const text = typeof value === "string" || typeof value === "number" || typeof value === "boolean" ? String(value) : JSON.stringify(value);
    out.push({ key, value: capText(text, META_CAP) });
  };
  push("seq", row.seq);
  push("id", row.id);
  push("action", row.action);
  push("actor", row.actor);
  push("entity", row.entity);
  push("entity_id", row.entity_id);
  push("ip", row.ip);
  const walk = (prefix: string, meta?: Record<string, unknown>) => {
    if (!meta) return;
    for (const [k, v] of Object.entries(meta)) {
      if (hideKey(k) || !META_KEYS.includes(k)) continue;
      push(`${prefix}.${k}`, v);
    }
  };
  walk("before", row.before);
  walk("after", row.after);
  return out;
}

export function activityQs(q?: ActivityQuery): string {
  const p = new URLSearchParams();
  if (q?.action) p.set("action", q.action);
  if (q?.actor) p.set("actor", q.actor);
  if (q?.entity) p.set("entity", q.entity);
  if (q?.ip) p.set("ip", q.ip);
  if (q?.since) p.set("since", q.since);
  if (q?.until) p.set("until", q.until);
  if (q?.limit) p.set("limit", String(q.limit));
  if (q?.before) p.set("before", String(q.before));
  const s = p.toString();
  return s ? `?${s}` : "";
}

export function uniqueField(rows: ActivityRecord[], key: "action" | "actor" | "entity" | "ip"): string[] {
  const seen = new Set<string>();
  for (const r of rows) {
    const v = (r[key] || "").trim();
    if (v) seen.add(v);
  }
  return [...seen].sort();
}

export function pageLabel(shown: number, total: number): { from: number; to: number; n: number } {
  return { from: total === 0 ? 0 : 1, to: shown, n: total };
}

export function activityFiltersActive(q: {
  action?: string;
  actor?: string;
  entity?: string;
  ip?: string;
  range?: string;
}): boolean {
  return Boolean(
    (q.action || "").trim() ||
      (q.actor || "").trim() ||
      (q.entity || "").trim() ||
      (q.ip || "").trim() ||
      (q.range && q.range !== "all"),
  );
}

export function classifyActivityList(input: {
  loading: boolean;
  loaded: boolean;
  error: unknown | null | undefined;
  itemCount: number;
}): PageState {
  return classifyPageState({
    loading: input.loading,
    loaded: input.loaded,
    error: input.error,
    itemCount: input.itemCount,
    keepStale: input.loaded && input.itemCount > 0,
  });
}

export function activityFilteredEmpty(state: PageState, filtersOn: boolean): boolean {
  return state.kind === "empty" && filtersOn;
}

export function activityActionsBlocked(kind: PageLoadKind): boolean {
  return inventoryBlocksMutation(kind);
}

export function activityCursorMeta(page: ActivityPage, stackLen: number): {
  hasPrev: boolean;
  hasNext: boolean;
  shown: number;
  total: number;
  before?: number;
  nextBefore?: number;
} {
  return {
    hasPrev: stackLen > 0,
    hasNext: Boolean(page.next_before),
    shown: page.records.length,
    total: page.total,
    before: page.before,
    nextBefore: page.next_before,
  };
}

export function localToRfc3339(v: string): string {
  const s = (v || "").trim();
  if (!s) return "";
  const d = new Date(s);
  if (Number.isNaN(d.getTime())) return "";
  return d.toISOString();
}
