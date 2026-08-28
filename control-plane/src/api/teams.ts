import { jsonFetch } from "./client";

export type Team = { id: string; name: string; lead_agent_id?: string; created_at: string };
export type TeamMember = { team_id: string; agent_id: string; role: string };
export type TeamTask = { id: string; team_id: string; title: string; status: string; assignee_agent_id?: string };
export type TeamMessage = { id: string; team_id: string; from_agent_id: string; body: string; created_at: string };
export type AgentLink = { from_agent_id: string; to_agent_id: string };
export type EvolutionSuggestion = { id: string; rule: string; text: string; status: string };
export type EvolutionGuardrails = { auto_adapt: boolean; min_runs: number; locked: string[] };

function idPath(id: string): string {
  return encodeURIComponent(id);
}

export const teamsApi = {
  list: () => jsonFetch<{ teams: Team[] }>("/api/teams"),
  get: (id: string) => jsonFetch<{ team: Team; members: TeamMember[] }>(`/api/teams/${idPath(id)}`),
  create: (body: { name: string; lead_agent_id: string }) =>
    jsonFetch<Team>("/api/teams", { method: "POST", body: JSON.stringify(body) }),
  update: (id: string, body: { name: string; lead_agent_id: string }) =>
    jsonFetch<Team>(`/api/teams/${idPath(id)}`, { method: "PUT", body: JSON.stringify(body) }),

  listMembers: (id: string) => jsonFetch<{ members: TeamMember[] }>(`/api/teams/${idPath(id)}/members`),
  addMember: (id: string, body: { agent_id: string; role: string }) =>
    jsonFetch<TeamMember>(`/api/teams/${idPath(id)}/members`, { method: "POST", body: JSON.stringify(body) }),

  listTasks: (id: string, status?: string) => {
    const qs = status ? `?status=${encodeURIComponent(status)}` : "";
    return jsonFetch<{ tasks: TeamTask[] }>(`/api/teams/${idPath(id)}/tasks${qs}`);
  },
  createTask: (id: string, body: { title: string; status?: string; assignee_agent_id?: string }) =>
    jsonFetch<TeamTask>(`/api/teams/${idPath(id)}/tasks`, { method: "POST", body: JSON.stringify(body) }),
  updateTask: (teamId: string, taskId: string, body: { title?: string; status?: string; assignee_agent_id?: string }) =>
    jsonFetch<TeamTask>(`/api/teams/${idPath(teamId)}/tasks/${idPath(taskId)}`, {
      method: "PATCH",
      body: JSON.stringify(body),
    }),

  listMessages: (id: string) => jsonFetch<{ messages: TeamMessage[] }>(`/api/teams/${idPath(id)}/messages`),
  createMessage: (id: string, body: { from_agent_id: string; body: string }) =>
    jsonFetch<TeamMessage>(`/api/teams/${idPath(id)}/messages`, { method: "POST", body: JSON.stringify(body) }),

  listLinks: (agentId: string) => jsonFetch<{ links: AgentLink[] }>(`/api/agents/${idPath(agentId)}/links`),
  addLink: (agentId: string, body: { to_agent_id: string; bidirectional?: boolean }) =>
    jsonFetch<{ links: AgentLink[] }>(`/api/agents/${idPath(agentId)}/links`, {
      method: "POST",
      body: JSON.stringify(body),
    }),

  listEvolution: (agentId: string) =>
    jsonFetch<{ suggestions: EvolutionSuggestion[]; guardrails?: EvolutionGuardrails }>(
      `/api/agents/${idPath(agentId)}/evolution`,
    ),
  patchEvolution: (agentId: string, body: { auto_adapt?: boolean; min_runs?: number }) =>
    jsonFetch<{ guardrails: EvolutionGuardrails }>(`/api/agents/${idPath(agentId)}/evolution`, {
      method: "PATCH",
      body: JSON.stringify(body),
    }),
  applyEvolution: (agentId: string, sid: string) =>
    jsonFetch<unknown>(`/api/agents/${idPath(agentId)}/evolution/${idPath(sid)}/apply`, {
      method: "POST",
      body: JSON.stringify({}),
    }),
};
