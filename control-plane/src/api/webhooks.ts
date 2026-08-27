import { jsonFetch } from "./client";

/** Returned once by POST /api/webhooks. There is no GET list. */
export type WebhookCreated = {
  id: string;
  token: string;
  token_prefix: string;
  hmac_key: string;
};

export const webhooksApi = {
  create: () => jsonFetch<WebhookCreated>("/api/webhooks", { method: "POST", body: JSON.stringify({}) }),
};
