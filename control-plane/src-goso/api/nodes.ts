import { jsonFetch } from "./client";

/** GET /api/nodes — pending vs paired devices. Never pairing codes or tokens. */
export type NodeDevice = {
  id: string;
  display: string;
  kind: string;
  status: string;
  health: string;
  requested_at?: string;
  expires_at?: string;
  approved_at?: string;
  last_seen?: string;
};

export type NodeList = {
  pending: NodeDevice[];
  paired: NodeDevice[];
};

export const nodesApi = {
  list: () => jsonFetch<NodeList>("/api/nodes"),
  approve: (id: string, confirm: string) =>
    jsonFetch<NodeDevice>(`/api/nodes/${encodeURIComponent(id)}/approve`, {
      method: "POST",
      body: JSON.stringify({ confirm }),
    }),
  deny: (id: string, confirm: string) =>
    jsonFetch<NodeDevice>(`/api/nodes/${encodeURIComponent(id)}/deny`, {
      method: "POST",
      body: JSON.stringify({ confirm }),
    }),
  revoke: (id: string, confirm: string) =>
    jsonFetch<NodeDevice>(`/api/nodes/${encodeURIComponent(id)}/revoke`, {
      method: "POST",
      body: JSON.stringify({ confirm }),
    }),
};
