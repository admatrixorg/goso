export const LIVE_CAP = 200;
export const SUMMARY_CAP = 400;
export const DETAIL_CAP = 200;

export const EVENT_TYPES = ["connector", "agent", "team", "task", "message", "agent_link"] as const;
export type EventType = (typeof EVENT_TYPES)[number];

export type GatewayEvent = {
  seq?: number;
  trace_id: string;
  type?: string;
  kind: string;
  action?: string;
  actor?: string;
  agent_id?: string;
  team_id?: string;
  entity?: string;
  connector?: string;
  tool?: string;
  ts: string;
  summary: string;
};

export type EventFilters = {
  type?: string;
  actor?: string;
  kind?: string;
  connector?: string;
};

export type EventDetail = { key: string; value: string };

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

const DETAIL_KEYS = [
  "type",
  "kind",
  "action",
  "actor",
  "agent_id",
  "team_id",
  "entity",
  "connector",
  "tool",
  "status",
  "latency_ms",
  "trace_id",
  "seq",
  "from_agent_id",
  "to_agent_id",
  "bytes",
  "enabled",
  "agent_key",
  "name",
  "role",
  "bidirectional",
  "pair",
  "task_id",
  "message_id",
  "team_id",
];

const TOKEN_SHAPE = /\b(sk-[A-Za-z0-9_-]{8,}|Bearer\s+[A-Za-z0-9._\-+=/]{8,})/i;

export function backoffDelay(attempt: number): number {
  const n = Math.max(0, attempt);
  const ms = 1000 * 2 ** Math.min(n, 4);
  return Math.min(ms, 15_000);
}

export function capText(s: string, n: number): string {
  const v = (s || "").replace(/\u0000/g, "");
  if (v.length <= n) return v;
  return `${v.slice(0, n)}…`;
}

function hideKey(k: string): boolean {
  const lk = k.toLowerCase();
  return SECRET_KEYS.some((sk) => lk === sk || lk.includes(sk)) || PAYLOAD_KEYS.some((pk) => lk === pk || lk.includes(pk));
}

function dropPayload(v: unknown): unknown {
  if (Array.isArray(v)) return v.map(dropPayload);
  if (v && typeof v === "object") {
    const out: Record<string, unknown> = {};
    for (const [k, child] of Object.entries(v as Record<string, unknown>)) {
      if (hideKey(k)) continue;
      out[k] = dropPayload(child);
    }
    return out;
  }
  if (typeof v === "string") return TOKEN_SHAPE.test(v) ? "[redacted]" : v;
  return v;
}

export function publicSummary(raw: string): string {
  let s = capText(String(raw || ""), SUMMARY_CAP * 2);
  s = s.replace(TOKEN_SHAPE, "[redacted]");
  try {
    const parsed = JSON.parse(s) as unknown;
    s = JSON.stringify(dropPayload(parsed));
  } catch {
    /* keep text */
  }
  return capText(s, SUMMARY_CAP);
}

export function asPublicEvent(raw: GatewayEvent | null | undefined): GatewayEvent | null {
  if (!raw || typeof raw !== "object") return null;
  const summary = publicSummary(raw.summary || "");
  if (publicHasSecrets({ ...raw, summary })) return null;
  return {
    seq: typeof raw.seq === "number" ? raw.seq : undefined,
    trace_id: String(raw.trace_id || ""),
    type: raw.type ? String(raw.type) : "connector",
    kind: String(raw.kind || ""),
    action: raw.action ? String(raw.action) : undefined,
    actor: raw.actor ? String(raw.actor) : undefined,
    agent_id: raw.agent_id ? String(raw.agent_id) : undefined,
    team_id: raw.team_id ? String(raw.team_id) : undefined,
    entity: raw.entity ? String(raw.entity) : undefined,
    connector: raw.connector ? String(raw.connector) : undefined,
    tool: raw.tool ? String(raw.tool) : undefined,
    ts: String(raw.ts || ""),
    summary,
  };
}

export function publicHasSecrets(row: unknown): boolean {
  const parts: string[] = [JSON.stringify(row ?? "")];
  if (row && typeof row === "object") {
    const rec = row as Record<string, unknown>;
    if (typeof rec.summary === "string") parts.push(rec.summary);
  }
  for (const s of parts) {
    if (/"body"\s*:/.test(s) || /"arguments"\s*:/.test(s) || /"tool_input"\s*:/.test(s) || /"tool_result"\s*:/.test(s)) return true;
    if (/"prompt"\s*:/.test(s) || /"messages"\s*:/.test(s)) return true;
    if (TOKEN_SHAPE.test(s)) return true;
    if (/"token"\s*:\s*"[^"]+"/i.test(s) && !/"token"\s*:\s*"\[redacted\]"/i.test(s)) return true;
  }
  return false;
}

export function parseDetail(row: GatewayEvent): EventDetail[] {
  const out: EventDetail[] = [];
  const push = (key: string, value: unknown) => {
    if (value == null || value === "") return;
    if (hideKey(key) || !DETAIL_KEYS.includes(key)) return;
    const text = typeof value === "string" || typeof value === "number" || typeof value === "boolean" ? String(value) : null;
    if (!text) return;
    if (TOKEN_SHAPE.test(text)) return;
    out.push({ key, value: capText(text, DETAIL_CAP) });
  };
  push("type", row.type);
  push("kind", row.kind);
  push("action", row.action);
  push("actor", row.actor);
  push("agent_id", row.agent_id);
  push("team_id", row.team_id);
  push("entity", row.entity);
  push("connector", row.connector);
  push("tool", row.tool);
  push("trace_id", row.trace_id);
  if (typeof row.seq === "number") push("seq", row.seq);
  try {
    const parsed = JSON.parse(row.summary || "") as unknown;
    if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
      for (const [k, v] of Object.entries(parsed as Record<string, unknown>)) {
        if (out.some((d) => d.key === k)) continue;
        push(k, v);
      }
    }
  } catch {
    /* summary is not an object */
  }
  return out;
}

export function eventKey(row: GatewayEvent, fallback = 0): string {
  if (typeof row.seq === "number" && row.seq > 0) return `seq:${row.seq}`;
  return `${row.trace_id || ""}:${row.kind || ""}:${row.ts || ""}:${fallback}`;
}

export function mergeLive(existing: GatewayEvent[], incoming: GatewayEvent, cap = LIVE_CAP): GatewayEvent[] {
  const next = asPublicEvent(incoming);
  if (!next) return existing;
  const key = eventKey(next);
  const out = [next, ...existing.filter((e) => eventKey(e) !== key)];
  return out.slice(0, cap);
}

export function applyFilters(rows: GatewayEvent[], f: EventFilters): GatewayEvent[] {
  return rows.filter((e) => {
    if (f.type && (e.type || "connector") !== f.type) return false;
    if (f.kind && e.kind !== f.kind) return false;
    if (f.connector && (e.connector || "") !== f.connector) return false;
    if (f.actor) {
      const a = f.actor;
      if (e.actor !== a && e.agent_id !== a && e.team_id !== a) return false;
    }
    return true;
  });
}

export function uniqueActors(rows: GatewayEvent[]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const e of rows) {
    for (const v of [e.actor, e.agent_id, e.team_id]) {
      const s = (v || "").trim();
      if (!s || seen.has(s)) continue;
      seen.add(s);
      out.push(s);
    }
  }
  return out;
}

export type StreamConn = "off" | "connecting" | "live" | "paused" | "reconnect" | "error";

export function parseSseBlock(raw: string): { event: string; id: string; data: string } {
  let event = "";
  let id = "";
  let data = "";
  for (const line of raw.split("\n")) {
    if (line.startsWith(":")) continue;
    if (line.startsWith("event:")) event = line.slice(6).trim();
    else if (line.startsWith("id:")) id = line.slice(3).trim();
    else if (line.startsWith("data:")) {
      const part = line.slice(5).trimStart();
      data = data ? `${data}\n${part}` : part;
    }
  }
  return { event, id, data };
}
