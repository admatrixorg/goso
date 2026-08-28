import { jsonFetch, type Connector } from "./client";

export type AgentTool = {
  name: string;
  connector: string;
  description?: string;
  requires_approval: boolean;
  enabled: boolean;
  configured?: boolean;
};

export const toolsApi = {
  list: (agentId: string) => jsonFetch<{ tools: AgentTool[] }>(`/api/agents/${agentId}/tools`),
  setEnabled: (agentId: string, name: string, enabled: boolean) =>
    jsonFetch<{ name: string; connector: string; enabled: boolean }>(`/api/agents/${agentId}/tools/${encodeURIComponent(name)}`, {
      method: "PATCH",
      body: JSON.stringify({ enabled }),
    }),
  patchConnector: (name: string, body: { enabled?: boolean; endpoint?: string; token?: string }) =>
    jsonFetch<Connector>(`/api/connectors/${encodeURIComponent(name)}`, {
      method: "PATCH",
      body: JSON.stringify(body),
    }),
};
