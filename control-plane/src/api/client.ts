// Gateway API client — talks to GOSO gateway (proxied via Vite or direct).
// Token is read from import.meta.env or localStorage; not hardcoded.

import { emptyGatewayStats, parseStatsBody, type GatewayStatsProbe } from "./stats";

export type { GatewayStatsProbe } from "./stats";

const GATEWAY_URL = (import.meta.env.VITE_GATEWAY_URL as string) || "";

function base(): string {
  return GATEWAY_URL.replace(/\/$/, "");
}

export const TENANT_STORAGE_KEY = "goso_tenant";

function authHeader(): Record<string, string> {
  const t = (import.meta.env.VITE_GOSO_ADMIN_TOKEN as string) || localStorage.getItem("goso_token") || "";
  if (t) return { Authorization: `Bearer ${t}` };
  return {};
}

function tenantHeader(): Record<string, string> {
  try {
    const t = (localStorage.getItem(TENANT_STORAGE_KEY) || "").trim();
    if (t) return { "X-Goso-Tenant": t };
  } catch {
    /* private mode */
  }
  return {};
}

export async function jsonFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const url = `${base()}${path}`;
  const res = await fetch(url, {
    ...init,
    headers: { "Content-Type": "application/json", ...authHeader(), ...tenantHeader(), ...(init?.headers as Record<string, string> | undefined) },
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`${res.status} ${text}`);
  }
  return (await res.json()) as T;
}

const HEALTH_TIMEOUT_MS = 5000;

/** GET /healthz without throwing — used by chrome. status 0 = network/timeout. */
export async function probeHealthz(signal?: AbortSignal): Promise<{ status: number; ok: boolean }> {
  const ctrl = new AbortController();
  const timer = setTimeout(() => ctrl.abort(), HEALTH_TIMEOUT_MS);
  const onAbort = () => ctrl.abort();
  if (signal) {
    if (signal.aborted) {
      clearTimeout(timer);
      return { status: 0, ok: false };
    }
    signal.addEventListener("abort", onAbort, { once: true });
  }
  try {
    const res = await fetch(`${base()}/healthz`, {
      method: "GET",
      cache: "no-store",
      headers: { ...authHeader(), ...tenantHeader() },
      signal: ctrl.signal,
    });
    let ok = false;
    if (res.status === 200) {
      try {
        const body = (await res.json()) as { ok?: unknown };
        ok = body.ok === true;
      } catch {
        ok = false;
      }
    } else {
      try {
        await res.text();
      } catch {
        /* ignore */
      }
    }
    return { status: res.status, ok };
  } catch {
    return { status: 0, ok: false };
  } finally {
    clearTimeout(timer);
    if (signal) signal.removeEventListener("abort", onAbort);
  }
}

/** GET /api/stats without throwing — used by chrome and Overview. status 0 = network/timeout. */
export async function probeStats(signal?: AbortSignal): Promise<GatewayStatsProbe> {
  const ctrl = new AbortController();
  const timer = setTimeout(() => ctrl.abort(), HEALTH_TIMEOUT_MS);
  const onAbort = () => ctrl.abort();
  if (signal) {
    if (signal.aborted) {
      clearTimeout(timer);
      return emptyGatewayStats(0);
    }
    signal.addEventListener("abort", onAbort, { once: true });
  }
  try {
    const res = await fetch(`${base()}/api/stats`, {
      method: "GET",
      cache: "no-store",
      headers: { ...authHeader(), ...tenantHeader() },
      signal: ctrl.signal,
    });
    if (!res.ok) {
      try {
        await res.text();
      } catch {
        /* ignore */
      }
      return emptyGatewayStats(res.status);
    }
    try {
      return parseStatsBody(await res.json(), res.status);
    } catch {
      return emptyGatewayStats(0);
    }
  } catch {
    return emptyGatewayStats(0);
  } finally {
    clearTimeout(timer);
    if (signal) signal.removeEventListener("abort", onAbort);
  }
}

export type ChatReply = { reply: string; session_id: string; trace?: unknown[] };
export type ChatBody = { session_id: string; message: string; prompt_mode?: string };

function isEventStream(ct: string | null): boolean {
  return (ct || "").toLowerCase().includes("text/event-stream");
}

async function chatJSON(body: ChatBody, onDelta?: (delta: string) => void): Promise<ChatReply> {
  const j = await jsonFetch<ChatReply>("/api/chat", { method: "POST", body: JSON.stringify(body) });
  if (j.reply && onDelta) onDelta(j.reply);
  return j;
}

async function readChatSSE(stream: ReadableStream<Uint8Array>, onDelta: (delta: string) => void): Promise<string> {
  const reader = stream.getReader();
  const dec = new TextDecoder();
  let buf = "";
  let reply = "";
  const flushBlock = (raw: string) => {
    let ev = "";
    let data = "";
    for (const line of raw.split("\n")) {
      if (line.startsWith(":")) continue;
      if (line.startsWith("event:")) {
        ev = line.slice(6).trim();
        continue;
      }
      if (line.startsWith("data:")) {
        const part = line.slice(5).trimStart();
        data = data ? `${data}\n${part}` : part;
      }
    }
    if (!data && !ev) return;
    if (ev === "error") {
      let msg = data;
      try {
        const j = JSON.parse(data) as { error?: string };
        if (j.error) msg = j.error;
      } catch {
        /* keep raw data */
      }
      throw new Error(msg);
    }
    if (data === "[DONE]") return;
    try {
      const j = JSON.parse(data) as { delta?: string };
      if (typeof j.delta === "string" && j.delta) {
        reply += j.delta;
        onDelta(j.delta);
      }
    } catch {
      /* ignore comments / keep-alives */
    }
  };
  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      buf += dec.decode(value, { stream: true }).replace(/\r\n/g, "\n").replace(/\r/g, "\n");
      let idx: number;
      while ((idx = buf.indexOf("\n\n")) >= 0) {
        const raw = buf.slice(0, idx);
        buf = buf.slice(idx + 2);
        flushBlock(raw);
      }
    }
    buf += dec.decode().replace(/\r\n/g, "\n").replace(/\r/g, "\n");
    if (buf.trim()) flushBlock(buf.replace(/\n+$/, ""));
  } finally {
    reader.releaseLock();
  }
  return reply;
}

/** Prefer SSE (Accept + stream:true). Fall back to JSON POST on 406 or non-stream responses. */
export async function chatStream(body: ChatBody, onDelta: (delta: string) => void): Promise<ChatReply> {
  const canStream = typeof fetch === "function" && typeof ReadableStream !== "undefined";
  if (!canStream) return chatJSON(body, onDelta);
  const res = await fetch(`${base()}/api/chat`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "text/event-stream",
      ...authHeader(),
      ...tenantHeader(),
    },
    body: JSON.stringify({ ...body, stream: true }),
  });
  if (res.status === 406) return chatJSON(body, onDelta);
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`${res.status} ${text}`);
  }
  if (!isEventStream(res.headers.get("content-type")) || !res.body) {
    const j = (await res.json()) as ChatReply;
    if (j.reply) onDelta(j.reply);
    return j;
  }
  const reply = await readChatSSE(res.body, onDelta);
  return { reply, session_id: body.session_id };
}

export const ORCHESTRATION_MODES = ["auto", "explicit", "manual"] as const;
export type OrchestrationMode = (typeof ORCHESTRATION_MODES)[number];
export const PROMPT_MODES = ["full", "task", "minimal", "none"] as const;
export type PromptMode = (typeof PROMPT_MODES)[number];
export type Agent = {
  id: string;
  agent_key: string;
  display_name: string;
  model?: string;
  llm_provider?: string;
  instructions?: string;
  orchestration_mode?: string;
  created_at: string;
};
export type Session = { id: string; agent_id: string; label?: string; prompt_mode?: string; created_at: string };
export type Message = { id: string; session_id: string; role: string; content: string; created_at: string };
export type Connector = {
  name: string;
  transport: string;
  endpoint: string;
  credential_ref?: string;
  schema_version?: string;
  enabled: boolean;
  health?: string;
  health_error?: string;
  created_at?: string;
  token_set?: boolean;
};
export type GatewayEvent = {
  trace_id: string;
  connector: string;
  tool: string;
  kind: string;
  ts: string;
  summary: string;
};
export type Approval = { approval_id: string; status: string; connector: string; tool: string };
export type Channel = {
  name: string;
  configured: boolean;
  missing?: boolean;
  env?: string;
  env_names?: string[];
};

export type TenantInfo = { tenant: string; multi_tenant: boolean };

export const api = {
  health: () => jsonFetch<{ ok: boolean; version: string }>("/healthz"),
  tenant: () => jsonFetch<TenantInfo>("/api/tenant"),
  listAgents: () => jsonFetch<{ agents: Agent[] }>("/api/agents"),
  getAgent: (id: string) => jsonFetch<Agent>(`/api/agents/${id}`),
  createAgent: (body: {
    agent_key: string;
    display_name: string;
    model?: string;
    llm_provider?: string;
    instructions?: string;
    orchestration_mode?: string;
  }) => jsonFetch<Agent>("/api/agents", { method: "POST", body: JSON.stringify(body) }),
  updateAgent: (
    id: string,
    body: { orchestration_mode?: string; model?: string; llm_provider?: string; instructions?: string },
  ) => jsonFetch<Agent>(`/api/agents/${id}`, { method: "PATCH", body: JSON.stringify(body) }),
  listSessions: () => jsonFetch<{ sessions: Session[] }>("/api/sessions"),
  createSession: (body: { agent_id: string; label?: string }) =>
    jsonFetch<Session>("/api/sessions", { method: "POST", body: JSON.stringify(body) }),
  updateSession: (id: string, body: { prompt_mode: string }) =>
    jsonFetch<Session>(`/api/sessions/${id}`, { method: "PATCH", body: JSON.stringify(body) }),
  listMessages: (sid: string) => jsonFetch<{ messages: Message[] }>(`/api/sessions/${sid}/messages`),
  chat: (body: ChatBody) => jsonFetch<ChatReply>("/api/chat", { method: "POST", body: JSON.stringify(body) }),
  chatStream,
  providers: () => jsonFetch<{ providers: Array<string | { name: string }> }>("/api/providers"),
  channels: () => jsonFetch<{ channels: Channel[] }>("/api/channels"),
  listConnectors: () => jsonFetch<{ connectors: Connector[] }>("/api/connectors"),
  createConnector: (body: {
    name: string;
    transport: string;
    endpoint: string;
    enabled?: boolean;
    credential_ref?: string;
    manifest_url?: string;
    timeout_ms?: number;
    retries?: number;
  }) => jsonFetch<Connector>("/api/connectors", { method: "POST", body: JSON.stringify(body) }),
  linkAgentConnector: (agentId: string, connector: string) =>
    jsonFetch<{ agent_id: string; connectors: string[] }>(`/api/agents/${agentId}/connectors`, {
      method: "POST",
      body: JSON.stringify({ connector }),
    }),
  listAgentConnectors: (agentId: string) =>
    jsonFetch<{ agent_id: string; connectors: string[] }>(`/api/agents/${agentId}/connectors`),
  listEvents: (q?: { kind?: string; connector?: string; limit?: number }) => {
    const p = new URLSearchParams();
    if (q?.kind) p.set("kind", q.kind);
    if (q?.connector) p.set("connector", q.connector);
    if (q?.limit) p.set("limit", String(q.limit));
    const qs = p.toString();
    return jsonFetch<{ events: GatewayEvent[] }>(`/api/events${qs ? `?${qs}` : ""}`);
  },
  decideApproval: (id: string, decision: "approve" | "reject") =>
    jsonFetch<Approval>(`/api/approvals/${id}/decision`, { method: "POST", body: JSON.stringify({ decision }) }),
};
