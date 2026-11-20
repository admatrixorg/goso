import { jsonFetch } from "./client";

/** Returned once by POST /api/webhooks and rotate. Secrets are never listed later. */
export type WebhookCreated = {
  id: string;
  token: string;
  token_prefix: string;
  hmac_key: string;
  name?: string;
  kind?: string;
  endpoint?: string;
  require_hmac?: boolean;
};

export type WebhookDelivery = {
  id?: string;
  status?: string;
  at?: string;
  http_status?: number;
};

/** GET /api/webhooks — hashed-at-rest view (no secrets). */
export type WebhookPublic = {
  id: string;
  token_prefix: string;
  name?: string;
  kind?: string;
  agent_id?: string;
  endpoint?: string;
  status?: string;
  require_hmac?: boolean;
  revoked?: boolean;
  secret_set?: boolean;
  last_delivery?: WebhookDelivery | null;
};

export type WebhookCreateBody = {
  name?: string;
  endpoint?: string;
  require_hmac?: boolean;
};

export const webhooksApi = {
  list: () => jsonFetch<{ webhooks: WebhookPublic[] }>("/api/webhooks"),
  create: (body: WebhookCreateBody = {}) =>
    jsonFetch<WebhookCreated>("/api/webhooks", { method: "POST", body: JSON.stringify(body) }),
  rotate: (id: string) => jsonFetch<WebhookCreated>(`/api/webhooks/${encodeURIComponent(id)}/rotate`, { method: "POST" }),
  revoke: (id: string) => jsonFetch<{ ok: boolean }>(`/api/webhooks/${encodeURIComponent(id)}`, { method: "DELETE" }),
  test: (id: string) => jsonFetch<WebhookDelivery>(`/api/webhooks/${encodeURIComponent(id)}/test`, { method: "POST" }),
  replay: (id: string) => jsonFetch<WebhookDelivery>(`/api/webhooks/${encodeURIComponent(id)}/replay`, { method: "POST" }),
};
