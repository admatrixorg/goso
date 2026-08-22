/** Custom tool endpoints: CRUD + invoke */

import type { HttpClient } from "../http-client.js";
import type {
  CustomTool,
  CreateCustomToolRequest,
  UpdateCustomToolRequest,
} from "../types.js";

export function customToolEndpoints(http: HttpClient) {
  return {
    listCustomTools: (agentId?: string) => {
      const qs = agentId ? `?agent_id=${agentId}` : "";
      return http.get<CustomTool[]>(`/v1/tools/custom${qs}`);
    },
    getCustomTool: (id: string) => http.get<CustomTool>(`/v1/tools/custom/${id}`),
    createCustomTool: (data: CreateCustomToolRequest) =>
      http.post<CustomTool>("/v1/tools/custom", data),
    updateCustomTool: (id: string, data: UpdateCustomToolRequest) =>
      http.put<CustomTool>(`/v1/tools/custom/${id}`, data),
    deleteCustomTool: (id: string) => http.del(`/v1/tools/custom/${id}`),
    invokeCustomTool: (id: string, args: Record<string, unknown>) =>
      http.post<{ result: string }>(`/v1/tools/invoke`, { tool_id: id, args }),
  };
}
