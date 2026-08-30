export type LinkDirection = "directed" | "bidirectional";

export const EVOLUTION_TEXT_CAP = 240;

export function teamDisplayName(t: { id: string; name?: string }): string {
  return (t.name || "").trim() || t.id;
}

export function filterTeams<T extends { id: string; name?: string; lead_agent_id?: string }>(teams: T[], query?: string): T[] {
  const q = (query || "").trim().toLowerCase();
  if (!q) return teams;
  return teams.filter((tm) => {
    const hay = `${tm.name || ""} ${tm.id} ${tm.lead_agent_id || ""}`.toLowerCase();
    return hay.includes(q);
  });
}

export function agentLabel(agents: { id: string; display_name?: string; agent_key?: string }[], id: string): string {
  const a = agents.find((x) => x.id === id);
  if (!a) return id;
  return (a.display_name || "").trim() || (a.agent_key || "").trim() || id;
}

export function isTeamLead(team: { lead_agent_id?: string } | undefined, agentId: string): boolean {
  return Boolean(team?.lead_agent_id) && team?.lead_agent_id === agentId;
}

export function validateTeamDraft(name: string, lead: string): "teams.needName" | "teams.needLead" | null {
  if (!name.trim()) return "teams.needName";
  if (!lead.trim()) return "teams.needLead";
  return null;
}

export function linkDirection(link: { bidirectional?: boolean }): LinkDirection {
  return link.bidirectional ? "bidirectional" : "directed";
}

export function linkArrow(dir: LinkDirection): string {
  return dir === "bidirectional" ? "↔" : "→";
}

export function namedConfirmTarget(expected: string, typed: string): boolean {
  return (typed || "").trim() === (expected || "").trim() && Boolean((expected || "").trim());
}

export type PublicAgentLink = { from_agent_id: string; to_agent_id: string; bidirectional?: boolean };

export function linkIdentity(link: PublicAgentLink): string {
  const a = (link.from_agent_id || "").trim();
  const b = (link.to_agent_id || "").trim();
  const dir = link.bidirectional ? "bi" : "dir";
  return `${a}|${b}|${dir}`;
}

/** Flatten per-agent link lists. Drops blanks. Does not invent status/description. */
export function mergeAgentLinks(groups: PublicAgentLink[][]): PublicAgentLink[] {
  const seen = new Set<string>();
  const out: PublicAgentLink[] = [];
  for (const group of groups) {
    for (const link of group) {
      const from = (link.from_agent_id || "").trim();
      const to = (link.to_agent_id || "").trim();
      if (!from || !to) continue;
      const row = { from_agent_id: from, to_agent_id: to, bidirectional: Boolean(link.bidirectional) };
      const id = linkIdentity(row);
      if (seen.has(id)) continue;
      seen.add(id);
      out.push(row);
    }
  }
  return out;
}

export type SettledLinkGroup =
  | { status: "fulfilled"; value: PublicAgentLink[] }
  | { status: "rejected"; reason: unknown };

export type AgentLinkLoadResult = {
  links: PublicAgentLink[];
  error: unknown | null;
  loaded: boolean;
};

/**
 * Flatten per-agent link fetches. A failed agent inventory must not become a
 * successful empty list — Promise.all([]) after that failure is not true-empty.
 */
export function resolveAgentLinkLoad(input: {
  agentInventoryError: unknown | null | undefined;
  agentInventoryLoaded: boolean;
  groups: SettledLinkGroup[];
}): AgentLinkLoadResult {
  if (input.agentInventoryError) {
    return { links: [], error: input.agentInventoryError, loaded: false };
  }
  if (!input.agentInventoryLoaded) {
    return { links: [], error: null, loaded: false };
  }
  const rows = mergeAgentLinks(input.groups.map((g) => (g.status === "fulfilled" ? g.value : [])));
  const fail = input.groups.find((g) => g.status === "rejected");
  return {
    links: rows,
    error: fail && fail.status === "rejected" ? fail.reason : null,
    loaded: true,
  };
}

export function filterLinks<T extends PublicAgentLink>(
  links: T[],
  query: string,
  labelFor: (id: string) => string,
): T[] {
  const q = (query || "").trim().toLowerCase();
  if (!q) return links;
  return links.filter((l) => {
    const hay = `${labelFor(l.from_agent_id)} ${labelFor(l.to_agent_id)} ${l.from_agent_id} ${l.to_agent_id}`.toLowerCase();
    return hay.includes(q);
  });
}

/** Truncate suggestion copy. Never surface a full system prompt dump. */
export function safeEvolutionText(text: string): string {
  const raw = (text || "").replace(/\s+/g, " ").trim();
  if (!raw) return "";
  if (/\binstructions\b/i.test(raw) && raw.length > 80) {
    return raw.slice(0, 80).trimEnd() + "…";
  }
  if (raw.length <= EVOLUTION_TEXT_CAP) return raw;
  return raw.slice(0, EVOLUTION_TEXT_CAP).trimEnd() + "…";
}

export function lockedFields(g?: { locked?: string[] } | null): string[] {
  return (g?.locked ?? []).map((x) => x.trim()).filter(Boolean);
}
