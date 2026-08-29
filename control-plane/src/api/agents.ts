export type AgentLite = {
  id: string;
  agent_key: string;
  display_name?: string;
  model?: string;
  llm_provider?: string;
  orchestration_mode?: string;
  enabled?: boolean;
  updated_at?: string;
  created_at?: string;
};

export type AgentStatusFilter = "active" | "inactive" | "";

export function agentDisplayName(a: { id: string; display_name?: string; agent_key?: string }): string {
  const name = (a.display_name || "").trim() || (a.agent_key || "").trim();
  return name || a.id;
}

export function isAgentActive(a: { enabled?: boolean }): boolean {
  return a.enabled !== false;
}

export function filterAgents<T extends AgentLite>(
  agents: T[],
  opts: { query?: string; status?: AgentStatusFilter; provider?: string } = {},
): T[] {
  const q = (opts.query || "").trim().toLowerCase();
  const status = (opts.status || "").trim();
  const provider = (opts.provider || "").trim();
  return agents.filter((a) => {
    if (status === "active" && !isAgentActive(a)) return false;
    if (status === "inactive" && isAgentActive(a)) return false;
    if (provider && (a.llm_provider || "") !== provider) return false;
    if (!q) return true;
    const hay = `${a.agent_key || ""} ${a.display_name || ""} ${a.id} ${a.model || ""} ${a.llm_provider || ""}`.toLowerCase();
    return hay.includes(q);
  });
}

export function uniqueProviders(agents: AgentLite[]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const a of agents) {
    const p = (a.llm_provider || "").trim();
    if (!p || seen.has(p)) continue;
    seen.add(p);
    out.push(p);
  }
  return out;
}

export function validateAgentKey(key: string, editing: boolean): "agents.needKey" | null {
  if (editing) return null;
  if (!key.trim()) return "agents.needKey";
  return null;
}

export function isConflictStatus(err: unknown): boolean {
  return /\b409\b/.test(String(err));
}

export function agentConflictKind(err: unknown): "conflict" | "lead" | "inactive" | "exists" | null {
  if (!isConflictStatus(err)) return null;
  const s = String(err);
  if (/team lead/i.test(s)) return "lead";
  if (/inactive/i.test(s)) return "inactive";
  if (/already exists/i.test(s)) return "exists";
  if (/was modified/i.test(s)) return "conflict";
  return "conflict";
}
