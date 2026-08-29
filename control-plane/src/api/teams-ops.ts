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
