export const SELECTED_SESSION_KEY = "goso_selected_session";
const PROMPT_MODES = ["full", "task", "minimal", "none"] as const;

export type SessionLite = {
  id: string;
  agent_id: string;
  label?: string;
  prompt_mode?: string;
  created_at?: string;
};

export type SelectedSession = { id: string; label: string };

export type MemoryStore = {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
  removeItem(key: string): void;
};

export type ChatStreamState = "idle" | "connecting" | "streaming" | "reconnect" | "error";

function fallbackStore(): MemoryStore | null {
  try {
    if (typeof localStorage === "undefined") return null;
    return localStorage;
  } catch {
    return null;
  }
}

export function parseSelectedSession(raw: string | null | undefined): SelectedSession | null {
  const v = (raw || "").trim();
  if (!v) return null;
  if (v.startsWith("{")) {
    try {
      const j = JSON.parse(v) as { id?: unknown; label?: unknown };
      const id = typeof j.id === "string" ? j.id.trim() : "";
      if (!id) return null;
      const label = typeof j.label === "string" ? j.label.trim() : "";
      return { id, label: label || id };
    } catch {
      return null;
    }
  }
  return { id: v, label: v };
}

export function readSelectedSession(store: MemoryStore | null = fallbackStore()): SelectedSession | null {
  if (!store) return null;
  try {
    return parseSelectedSession(store.getItem(SELECTED_SESSION_KEY));
  } catch {
    return null;
  }
}

export function writeSelectedSession(sel: SelectedSession, store: MemoryStore | null = fallbackStore()): void {
  const id = sel.id.trim();
  if (!id || !store) return;
  const label = sel.label.trim() || id;
  try {
    store.setItem(SELECTED_SESSION_KEY, JSON.stringify({ id, label }));
  } catch {
    /* quota / private mode */
  }
}

export function clearSelectedSession(store: MemoryStore | null = fallbackStore()): void {
  if (!store) return;
  try {
    store.removeItem(SELECTED_SESSION_KEY);
  } catch {
    /* ignore */
  }
}

export function sessionDisplayName(s: { id: string; label?: string }): string {
  const label = (s.label || "").trim();
  return label || s.id;
}

/** Last-activity field available on session list JSON. No message_count in this API. */
export function sessionActivityAt(s: { created_at?: string }): string {
  return (s.created_at || "").trim();
}

export function filterSessions<T extends SessionLite>(
  sessions: T[],
  opts: { query?: string; agentId?: string } = {},
): T[] {
  const q = (opts.query || "").trim().toLowerCase();
  const agent = (opts.agentId || "").trim();
  return sessions.filter((s) => {
    if (agent && s.agent_id !== agent) return false;
    if (!q) return true;
    const hay = `${s.label || ""} ${s.id} ${s.agent_id}`.toLowerCase();
    return hay.includes(q);
  });
}

export function agentLabel(
  agents: Array<{ id: string; display_name?: string; agent_key?: string }>,
  agentId: string,
): string {
  const a = agents.find((x) => x.id === agentId);
  if (!a) return agentId;
  const name = (a.display_name || a.agent_key || "").trim();
  return name || agentId;
}

export function normalizePromptMode(mode?: string): string {
  const v = (mode || "").trim().toLowerCase();
  return (PROMPT_MODES as readonly string[]).includes(v) ? v : "full";
}

export function streamReconnectDelayMs(attempt: number): number {
  const n = Math.max(0, Math.trunc(Number.isFinite(attempt) ? attempt : 0));
  return Math.min(400 * 2 ** Math.min(n, 4), 4000);
}

export function isGoneStatus(err: unknown): boolean {
  return /\b404\b/.test(String(err));
}
