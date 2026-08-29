import { jsonFetch } from "./client";
import { asPublic, type ApiKey, type ApiKeyCreated } from "./apikeys-ops";

export type { ApiKey, ApiKeyCreated } from "./apikeys-ops";

export type ApiKeyList = { keys: ApiKey[] };

export const apiKeysApi = {
  list: async (q?: string): Promise<ApiKeyList> => {
    const qs = q && q.trim() ? `?q=${encodeURIComponent(q.trim())}` : "";
    const j = await jsonFetch<ApiKeyList>(`/api/api-keys${qs}`);
    return { keys: asPublic(j.keys) };
  },
  get: async (id: string): Promise<ApiKey> => {
    const row = await jsonFetch<ApiKey>(`/api/api-keys/${encodeURIComponent(id)}`);
    const pub = asPublic([row])[0];
    if (!pub) throw new Error("secret-shaped payload");
    return pub;
  },
  create: (body: { name: string; tenant_id?: string; scopes: string[]; expires_at?: string }) =>
    jsonFetch<ApiKeyCreated>("/api/api-keys", { method: "POST", body: JSON.stringify(body) }),
  revoke: (id: string, confirm: string) =>
    jsonFetch<ApiKey>(`/api/api-keys/${encodeURIComponent(id)}/revoke`, {
      method: "POST",
      body: JSON.stringify({ confirm }),
    }),
};
