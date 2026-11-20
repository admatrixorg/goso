import { jsonFetch, type Connector } from "./client";
import type { ConnectorTestResult, ConnectorWrite } from "./function-ops";

export type { ConnectorInfo } from "./function-ops";
export {
  configuredLabel,
  connectorWriteBody,
  formatConnectorTest,
  isConnectorEnabled,
  isConnectorEnvOwned,
  normalizeTransport,
} from "./function-ops";

export type AgentTool = {
  name: string;
  connector: string;
  description?: string;
  requires_approval: boolean;
  enabled: boolean;
  configured?: boolean;
  granted?: boolean;
};

export const toolsApi = {
  list: (agentId: string) => jsonFetch<{ tools: AgentTool[] }>(`/api/agents/${agentId}/tools`),
  setEnabled: (agentId: string, name: string, enabled: boolean) =>
    jsonFetch<{ name: string; connector: string; enabled: boolean; granted?: boolean }>(
      `/api/agents/${agentId}/tools/${encodeURIComponent(name)}`,
      {
        method: "PATCH",
        body: JSON.stringify({ enabled }),
      },
    ),
  createConnector: (body: ConnectorWrite) =>
    jsonFetch<Connector>("/api/connectors", { method: "POST", body: JSON.stringify(body) }),
  patchConnector: (name: string, body: ConnectorWrite) =>
    jsonFetch<Connector>(`/api/connectors/${encodeURIComponent(name)}`, {
      method: "PATCH",
      body: JSON.stringify(body),
    }),
  testConnector: (name: string) =>
    jsonFetch<ConnectorTestResult>(`/api/connectors/${encodeURIComponent(name)}/test`, {
      method: "POST",
      body: "{}",
    }),
};
