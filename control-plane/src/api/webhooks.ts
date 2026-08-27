import { jsonFetch } from "./client";

/** Returned once by POST /api/webhooks. Secrets are never listed later. */
export type WebhookCreated = {
  id: string;
  token: string;
  token_prefix: string;
  hmac_key: string;
};

/** GET /api/webhooks — hashed-at-rest view (no secrets). */
export type WebhookPublic = {
  id: string;
  token_prefix: string;
};

export const webhooksApi = {
  list: () => jsonFetch<{ webhooks: WebhookPublic[] }>("/api/webhooks"),
  create: () => jsonFetch<WebhookCreated>("/api/webhooks", { method: "POST", body: JSON.stringify({}) }),
};
