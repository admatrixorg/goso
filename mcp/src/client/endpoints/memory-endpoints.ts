/** Memory document endpoints: CRUD */

import type { HttpClient } from "../http-client.js";
import type { MemoryDocument } from "../types.js";

export function memoryEndpoints(http: HttpClient) {
  return {
    listMemoryDocs: (agentId: string, userId?: string) => {
      const qs = new URLSearchParams({ agent_id: agentId });
      if (userId) qs.set("user_id", userId);
      return http.get<MemoryDocument[]>(`/v1/memory?${qs.toString()}`);
    },
    getMemoryDoc: (id: string) => http.get<MemoryDocument>(`/v1/memory/${id}`),
    createMemoryDoc: (agentId: string, path: string, content: string) =>
      http.post<MemoryDocument>("/v1/memory", { agent_id: agentId, path, content }),
    deleteMemoryDoc: (id: string) => http.del(`/v1/memory/${id}`),
  };
}
