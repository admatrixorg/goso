/** MCP Server management endpoints: CRUD + grants */

import type { HttpClient } from "../http-client.js";
import type {
  McpServerEntry,
  CreateMcpServerRequest,
  UpdateMcpServerRequest,
} from "../types.js";

export function mcpServerEndpoints(http: HttpClient) {
  return {
    listMcpServers: () => http.get<McpServerEntry[]>("/v1/mcp/servers"),
    getMcpServer: (id: string) => http.get<McpServerEntry>(`/v1/mcp/servers/${id}`),
    createMcpServer: (data: CreateMcpServerRequest) =>
      http.post<McpServerEntry>("/v1/mcp/servers", data),
    updateMcpServer: (id: string, data: UpdateMcpServerRequest) =>
      http.put<McpServerEntry>(`/v1/mcp/servers/${id}`, data),
    deleteMcpServer: (id: string) => http.del(`/v1/mcp/servers/${id}`),
    grantMcpServerAgent: (serverId: string, agentId: string) =>
      http.post(`/v1/mcp/servers/${serverId}/grants/agent`, { agent_id: agentId }),
    grantMcpServerUser: (serverId: string, userId: string) =>
      http.post(`/v1/mcp/servers/${serverId}/grants/user`, { user_id: userId }),
  };
}
