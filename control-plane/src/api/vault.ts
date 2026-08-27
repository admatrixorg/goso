import { jsonFetch } from "./client";

export type VaultDoc = {
  id: string;
  title: string;
  path: string;
  sha256: string;
  mtime: string;
  body?: string;
};
export type VaultLink = { from_id: string; to_id?: string; raw: string };
export type VaultSearchHit = { id: string; title: string; path: string; snippet: string };
export type VaultSyncResult = { upserted: number; skipped: number; deleted: number };

function asHits(j: unknown): VaultSearchHit[] {
  if (Array.isArray(j)) return j as VaultSearchHit[];
  if (j && typeof j === "object") {
    const inner = (j as { hits?: unknown }).hits;
    if (Array.isArray(inner)) return inner as VaultSearchHit[];
  }
  return [];
}

function idPath(id: string): string {
  return encodeURIComponent(id);
}

export const vaultApi = {
  list: () => jsonFetch<{ docs: VaultDoc[] }>("/api/vault/docs"),
  get: (id: string) => jsonFetch<VaultDoc>(`/api/vault/docs/${idPath(id)}`),
  put: (body: { title: string; body: string }) =>
    jsonFetch<VaultDoc>("/api/vault/docs", { method: "PUT", body: JSON.stringify(body) }),
  links: (id: string) =>
    jsonFetch<{ outbound: VaultLink[]; inbound: VaultLink[] }>(`/api/vault/docs/${idPath(id)}/links`),
  search: async (q: string) => asHits(await jsonFetch<unknown>(`/api/vault/search?q=${encodeURIComponent(q)}`)),
  sync: () => jsonFetch<VaultSyncResult>("/api/vault/sync", { method: "POST", body: JSON.stringify({}) }),
};
