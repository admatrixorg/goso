// Gateway API client — talks to GOSO gateway (proxied via Vite or direct).
// Token is read from import.meta.env or localStorage; not hardcoded.

const GATEWAY_URL = (import.meta.env.VITE_GATEWAY_URL as string) || "";

function base(): string {
  return GATEWAY_URL.replace(/\/$/, "");
}

function authHeader(): Record<string, string> {
  const t = (import.meta.env.VITE_GOSO_ADMIN_TOKEN as string) || localStorage.getItem("goso_token") || "";
  if (t) return { Authorization: `Bearer ${t}` };
  return {};
}

export async function jsonFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const url = `${base()}${path}`;
  const res = await fetch(url, {
    ...init,
    headers: { "Content-Type": "application/json", ...authHeader(), ...(init?.headers as Record<string, string> | undefined) },
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`${res.status} ${text}`);
  }
  return (await res.json()) as T;
}

export const ORCHESTRATION_MODES = ["auto", "explicit", "manual"] as const;
export type OrchestrationMode = (typeof ORCHESTRATION_MODES)[number];
export type Agent = {
  id: string;
  agent_key: string;
  display_name: string;
  model?: string;
  instructions?: string;
  orchestration_mode?: string;
  created_at: string;
};
export type Session = { id: string; agent_id: string; label?: string; created_at: string };
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
export type Channel = { name: string; configured: boolean };

export const api = {
  health: () => jsonFetch<{ ok: boolean; version: string }>("/healthz"),
  listAgents: () => jsonFetch<{ agents: Agent[] }>("/api/agents"),
  getAgent: (id: string) => jsonFetch<Agent>(`/api/agents/${id}`),
  createAgent: (body: { agent_key: string; display_name: string; model?: string }) =>
    jsonFetch<Agent>("/api/agents", { method: "POST", body: JSON.stringify(body) }),
  updateAgent: (id: string, body: { orchestration_mode?: string; model?: string; instructions?: string }) =>
    jsonFetch<Agent>(`/api/agents/${id}`, { method: "PATCH", body: JSON.stringify(body) }),
  listSessions: () => jsonFetch<{ sessions: Session[] }>("/api/sessions"),
  createSession: (body: { agent_id: string; label?: string }) =>
    jsonFetch<Session>("/api/sessions", { method: "POST", body: JSON.stringify(body) }),
  listMessages: (sid: string) => jsonFetch<{ messages: Message[] }>(`/api/sessions/${sid}/messages`),
  chat: (body: { session_id: string; message: string }) =>
    jsonFetch<{ reply: string; session_id: string; trace?: unknown[] }>("/api/chat", { method: "POST", body: JSON.stringify(body) }),
  providers: () => jsonFetch<{ providers: string[] }>("/api/providers"),
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
