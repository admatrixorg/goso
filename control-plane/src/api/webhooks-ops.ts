import type { WebhookCreated, WebhookDelivery, WebhookPublic } from "./webhooks";

export const INBOUND_ENDPOINT = "/api/webhooks/llm";

export type LastSecret = {
  id: string;
  token_prefix: string;
  token?: string;
  hmac_key?: string;
  note?: string;
};

export function asCreated(j: WebhookCreated): LastSecret {
  return {
    id: typeof j.id === "string" ? j.id : "",
    token_prefix: typeof j.token_prefix === "string" ? j.token_prefix : "",
    token: typeof j.token === "string" ? j.token : undefined,
    hmac_key: typeof j.hmac_key === "string" ? j.hmac_key : undefined,
  };
}

export function asPublic(rows: WebhookPublic[] | undefined): WebhookPublic[] {
  return (rows ?? [])
    .map((w) => ({
      id: typeof w?.id === "string" ? w.id : "",
      token_prefix: typeof w?.token_prefix === "string" ? w.token_prefix : "",
      name: typeof w?.name === "string" ? w.name : "",
      kind: typeof w?.kind === "string" ? w.kind : "llm",
      agent_id: typeof w?.agent_id === "string" ? w.agent_id : "",
      endpoint: webhookEndpoint(w),
      status: webhookStatus(w),
      require_hmac: Boolean(w?.require_hmac),
      revoked: Boolean(w?.revoked),
      secret_set: Boolean(w?.secret_set),
      last_delivery: asDelivery(w?.last_delivery),
    }))
    .filter((w) => w.id);
}

export function asDelivery(d: WebhookDelivery | null | undefined): WebhookDelivery | null {
  if (!d || typeof d !== "object") return null;
  const id = typeof d.id === "string" ? d.id : "";
  const status = typeof d.status === "string" ? d.status : "";
  const at = typeof d.at === "string" ? d.at : "";
  const httpStatus = typeof d.http_status === "number" ? d.http_status : undefined;
  if (!id && !status && !at) return null;
  return { id, status, at, http_status: httpStatus };
}

export function webhookStatus(row?: Pick<WebhookPublic, "status" | "revoked" | "last_delivery"> | null): string {
  if (row?.revoked || row?.status === "revoked") return "revoked";
  const last = row?.last_delivery?.status || "";
  if (last === "failed" || last === "dead") return "failing";
  if (row?.status === "active" || row?.status === "failing") return row.status;
  return "active";
}

export function webhookEndpoint(row?: Pick<WebhookPublic, "endpoint"> | null): string {
  const ep = (row?.endpoint || "").trim();
  return ep || INBOUND_ENDPOINT;
}

export function lastDeliveryLabel(row?: Pick<WebhookPublic, "last_delivery"> | null): string {
  const d = row?.last_delivery;
  if (!d || (!d.status && !d.at)) return "";
  const status = (d.status || "").trim();
  const at = (d.at || "").trim();
  if (status && at) return `${status} · ${at}`;
  return status || at;
}

export function listTargetName(row: { name?: string; id?: string; endpoint?: string }): string {
  const name = (row.name || "").trim();
  if (name) return name;
  const id = (row.id || "").trim();
  if (id) return id;
  return (row.endpoint || "").trim() || "webhook";
}

export function hideCopiedSecret(last: LastSecret, kind: "token" | "hmac"): LastSecret {
  return {
    ...last,
    token: kind === "token" ? undefined : last.token,
    hmac_key: kind === "hmac" ? undefined : last.hmac_key,
  };
}

/** True when a GET-shaped object still carries a signing secret. */
export function publicHasSecrets(row: Record<string, unknown> | null | undefined): boolean {
  if (!row) return false;
  if (typeof row.token === "string" && row.token.length > 0) return true;
  if (typeof row.hmac_key === "string" && row.hmac_key.length > 0) return true;
  return false;
}

export function canTestOrReplay(row?: Pick<WebhookPublic, "endpoint" | "status" | "revoked"> | null): boolean {
  if (!row || webhookStatus(row) === "revoked") return false;
  const ep = webhookEndpoint(row);
  return ep !== INBOUND_ENDPOINT && /^https?:\/\//i.test(ep);
}

export function canReplay(row?: Pick<WebhookPublic, "endpoint" | "status" | "revoked" | "last_delivery"> | null): boolean {
  if (!canTestOrReplay(row)) return false;
  const d = row?.last_delivery;
  return Boolean(d && (d.id || d.status || d.at));
}
