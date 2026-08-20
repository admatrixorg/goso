/** Agent endpoints: CRUD, context files, links, shares */

import type { HttpClient } from "../http-client.js";
import type {
  Agent,
  CreateAgentRequest,
  UpdateAgentRequest,
  AgentFile,
  AgentLink,
  GrantRequest,
} from "../types.js";

export function agentEndpoints(http: HttpClient) {
  return {
    listAgents: (params?: { include_deleted?: boolean }) => {
      const qs = params?.include_deleted ? "?include_deleted=true" : "";
      return http.get<Agent[]>(`/v1/agents${qs}`);
    },
    getAgent: (id: string) => http.get<Agent>(`/v1/agents/${id}`),
    createAgent: (data: CreateAgentRequest) => http.post<Agent>("/v1/agents", data),
    updateAgent: (id: string, data: UpdateAgentRequest) =>
      http.put<Agent>(`/v1/agents/${id}`, data),
    deleteAgent: (id: string) => http.del(`/v1/agents/${id}`),

    // Context files
    listAgentFiles: (agentId: string) =>
      http.get<AgentFile[]>(`/v1/agents/${agentId}/files`),
    getAgentFile: (agentId: string, path: string) =>
      http.get<AgentFile>(`/v1/agents/${agentId}/files/${encodeURIComponent(path)}`),
    setAgentFile: (agentId: string, path: string, content: string) =>
      http.put<AgentFile>(`/v1/agents/${agentId}/files/${encodeURIComponent(path)}`, {
        content,
      }),
    deleteAgentFile: (agentId: string, path: string) =>
      http.del(`/v1/agents/${agentId}/files/${encodeURIComponent(path)}`),

    // Links (delegation)
    listAgentLinks: (agentId: string) =>
      http.get<AgentLink[]>(`/v1/agents/${agentId}/links`),
    setAgentLink: (agentId: string, targetAgentId: string, description: string) =>
      http.put<AgentLink>(`/v1/agents/${agentId}/links/${targetAgentId}`, {
        description,
      }),
    removeAgentLink: (agentId: string, targetAgentId: string) =>
      http.del(`/v1/agents/${agentId}/links/${targetAgentId}`),

    // Shares
    shareAgent: (agentId: string, grant: GrantRequest) =>
      http.post(`/v1/agents/${agentId}/shares`, grant),
  };
}
