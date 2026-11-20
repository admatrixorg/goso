import { jsonFetch } from "./client";

/** GET /api/pending-messages — counts and metadata only. Never payloads. */
export type PendingGroup = {
  id: string;
  channel: string;
  dest: string;
  agent_id?: string;
  agent?: string;
  count: number;
  oldest_at?: string;
  newest_at?: string;
  age_ms?: number;
  compacted?: boolean;
  compacting?: boolean;
  compacted_from?: number;
};

export const pendingApi = {
  list: () => jsonFetch<{ groups: PendingGroup[] }>("/api/pending-messages"),
  compact: (id: string, confirm: string) =>
    jsonFetch<PendingGroup>(`/api/pending-messages/${encodeURIComponent(id)}/compact`, {
      method: "POST",
      body: JSON.stringify({ confirm }),
    }),
  clear: (id: string, confirm: string) =>
    jsonFetch<{ ok: boolean; id: string }>(`/api/pending-messages/${encodeURIComponent(id)}/clear`, {
      method: "POST",
      body: JSON.stringify({ confirm }),
    }),
};
