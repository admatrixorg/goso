/** Session endpoints: list, preview, delete, reset, label */

import type { HttpClient } from "../http-client.js";
import type { Session, SessionPreview } from "../types.js";

export function sessionEndpoints(http: HttpClient) {
  return {
    listSessions: (params?: { agent_id?: string; limit?: number }) => {
      const qs = new URLSearchParams();
      if (params?.agent_id) qs.set("agent_id", params.agent_id);
      if (params?.limit) qs.set("limit", String(params.limit));
      const query = qs.toString();
      return http.get<Session[]>(`/v1/sessions${query ? `?${query}` : ""}`);
    },
    previewSession: (sessionKey: string, limit?: number) => {
      const qs = limit ? `?limit=${limit}` : "";
      return http.get<SessionPreview>(
        `/v1/sessions/${encodeURIComponent(sessionKey)}/preview${qs}`
      );
    },
    deleteSession: (sessionKey: string) =>
      http.del(`/v1/sessions/${encodeURIComponent(sessionKey)}`),
    resetSession: (sessionKey: string) =>
      http.post(`/v1/sessions/${encodeURIComponent(sessionKey)}/reset`),
    labelSession: (sessionKey: string, label: string) =>
      http.patch(`/v1/sessions/${encodeURIComponent(sessionKey)}`, { label }),
  };
}
