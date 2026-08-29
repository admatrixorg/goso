import { jsonFetch } from "./client";

export type ChannelRow = {
  name: string;
  configured: boolean;
  missing: boolean;
  env: string;
  env_names: string[];
  health?: string;
  transport?: string;
  secret_set?: boolean;
  bound_agent_id?: string;
  dm_policy?: string;
  group_policy?: string;
  require_mention?: boolean;
  allow_from?: string[];
  allow_from_count?: number;
  phase?: number;
  last_error?: string;
  enabled?: boolean;
};

export type ChannelPairingItem = {
  id: string;
  channel: string;
  sender_id: string;
  status: string;
  expires_at?: string;
};

export const channelsApi = {
  list: () => jsonFetch<{ channels: ChannelRow[]; lite?: boolean }>("/api/channels"),
  patch: (name: string, body: Record<string, unknown>) =>
    jsonFetch<{ ok: boolean; name: string }>(`/api/channels/${encodeURIComponent(name)}`, {
      method: "PATCH",
      body: JSON.stringify(body),
    }),
  qr: () => jsonFetch<{ status: string; expires_at?: string }>("/api/channels/zalo-personal/qr"),
  logoutPersonal: () => jsonFetch<{ ok: boolean }>("/api/channels/zalo-personal/logout", { method: "POST", body: "{}" }),
  pairingList: () => jsonFetch<{ items: ChannelPairingItem[] }>("/api/channel-pairing"),
  pairingApprove: (id: string) =>
    jsonFetch<{ ok: boolean }>(`/api/channel-pairing/${encodeURIComponent(id)}/approve`, { method: "POST", body: "{}" }),
  pairingDeny: (id: string) =>
    jsonFetch<{ ok: boolean }>(`/api/channel-pairing/${encodeURIComponent(id)}/deny`, { method: "POST", body: "{}" }),
};
