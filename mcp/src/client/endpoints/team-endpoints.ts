/** Team endpoints: CRUD against gateway /api/teams (SPEC 038). */

import type { HttpClient } from "../http-client.js";
import type { Team, CreateTeamRequest, UpdateTeamRequest } from "../types.js";

type TeamList = { teams?: Team[] } | Team[];

function unwrapTeams(r: TeamList): Team[] {
  if (Array.isArray(r)) return r;
  return r.teams ?? [];
}

export function teamEndpoints(http: HttpClient) {
  return {
    listTeams: async () => unwrapTeams(await http.get<TeamList>("/api/teams")),
    getTeam: async (id: string) => {
      const r = await http.get<Team | { team: Team }>(`/api/teams/${id}`);
      return "team" in r && r.team ? r.team : (r as Team);
    },
    createTeam: (data: CreateTeamRequest) =>
      http.post<Team>("/api/teams", {
        name: data.name,
        lead_agent_id: data.member_agent_ids?.[0],
      }),
    updateTeam: (id: string, data: UpdateTeamRequest) =>
      http.put<Team>(`/api/teams/${id}`, { name: data.name }),
    deleteTeam: (id: string) => http.del(`/api/teams/${id}`),
  };
}
