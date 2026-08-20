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

async function jsonFetch<T>(path: string, init?: RequestInit): Promise<T> {
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

export type Agent = { id: string; agent_key: string; display_name: string; model?: string; created_at: string };
export type Session = { id: string; agent_id: string; label?: string; created_at: string };
export type Message = { id: string; session_id: string; role: string; content: string; created_at: string };

export const api = {
  health: () => jsonFetch<{ ok: boolean; version: string }>("/healthz"),
  listAgents: () => jsonFetch<{ agents: Agent[] }>("/api/agents"),
  getAgent: (id: string) => jsonFetch<Agent>(`/api/agents/${id}`),
  createAgent: (body: { agent_key: string; display_name: string; model?: string }) =>
    jsonFetch<Agent>("/api/agents", { method: "POST", body: JSON.stringify(body) }),
  listSessions: () => jsonFetch<{ sessions: Session[] }>("/api/sessions"),
  createSession: (body: { agent_id: string; label?: string }) =>
    jsonFetch<Session>("/api/sessions", { method: "POST", body: JSON.stringify(body) }),
  listMessages: (sid: string) => jsonFetch<{ messages: Message[] }>(`/api/sessions/${sid}/messages`),
  chat: (body: { session_id: string; message: string }) =>
    jsonFetch<{ reply: string; session_id: string }>("/api/chat", { method: "POST", body: JSON.stringify(body) }),
  providers: () => jsonFetch<{ providers: string[] }>("/api/providers"),
  channels: () => jsonFetch<{ channels: string[] }>("/api/channels"),
};
