/** Team endpoints: CRUD */

import type { HttpClient } from "../http-client.js";
import type { Team, CreateTeamRequest, UpdateTeamRequest } from "../types.js";

export function teamEndpoints(http: HttpClient) {
  return {
    listTeams: () => http.get<Team[]>("/v1/teams"),
    getTeam: (id: string) => http.get<Team>(`/v1/teams/${id}`),
    createTeam: (data: CreateTeamRequest) => http.post<Team>("/v1/teams", data),
    updateTeam: (id: string, data: UpdateTeamRequest) =>
      http.put<Team>(`/v1/teams/${id}`, data),
    deleteTeam: (id: string) => http.del(`/v1/teams/${id}`),
  };
}
