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
  from_env?: boolean;
  writable?: string[];
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

export type ChannelSecretPut = {
  ok: boolean;
  name: string;
  secret_set?: boolean;
  from_env?: boolean;
  written?: string[];
};

export type ChannelTestResult = {
  ok: boolean;
  name: string;
  health?: string;
  error?: string;
  secret_set?: boolean;
};

export const channelsApi = {
  list: () => jsonFetch<{ channels: ChannelRow[]; lite?: boolean }>("/api/channels"),
  putSecrets: (name: string, body: Record<string, string>) =>
    jsonFetch<ChannelSecretPut>(`/api/channels/${encodeURIComponent(name)}/secrets`, {
      method: "PUT",
      body: JSON.stringify(body),
    }),
  test: (name: string) =>
    jsonFetch<ChannelTestResult>(`/api/channels/${encodeURIComponent(name)}/test`, {
      method: "POST",
      body: "{}",
    }),
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
