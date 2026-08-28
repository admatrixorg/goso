import { jsonFetch } from "./client";

/** Returned once by POST /api/webhooks and rotate. Secrets are never listed later. */
export type WebhookCreated = {
  id: string;
  token: string;
  token_prefix: string;
  hmac_key: string;
  name?: string;
  kind?: string;
  require_hmac?: boolean;
};

/** GET /api/webhooks — hashed-at-rest view (no secrets). */
export type WebhookPublic = {
  id: string;
  token_prefix: string;
  name?: string;
  kind?: string;
  agent_id?: string;
  require_hmac?: boolean;
  revoked?: boolean;
};

export const webhooksApi = {
  list: () => jsonFetch<{ webhooks: WebhookPublic[] }>("/api/webhooks"),
  create: () => jsonFetch<WebhookCreated>("/api/webhooks", { method: "POST", body: JSON.stringify({}) }),
  rotate: (id: string) => jsonFetch<WebhookCreated>(`/api/webhooks/${encodeURIComponent(id)}/rotate`, { method: "POST" }),
  revoke: (id: string) => jsonFetch<{ ok: boolean }>(`/api/webhooks/${encodeURIComponent(id)}`, { method: "DELETE" }),
};
