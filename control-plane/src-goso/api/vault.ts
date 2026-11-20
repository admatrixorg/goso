import { jsonFetch } from "./client";
import { GRAPH_NODE_CAP, normalizeGraph, type VaultGraphLite, type VaultHealthLite } from "./vault-ops";

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
export type VaultHealth = VaultHealthLite;
export type VaultGraph = VaultGraphLite;

export {
  BODY_CAP,
  boundNeighborhood,
  capRows,
  classifyDoc,
  classifyVaultDocs,
  classifyVaultHealth,
  filterVaultDocs,
  formatMtime,
  GRAPH_NODE_CAP,
  inventoryOptionsFromDocs,
  isStaleHealth,
  LIST_CAP,
  normalizeGraph,
  parseVaultFrontmatter,
  plainVaultBody,
  shortHash,
  uniqueField,
  vaultFilteredEmpty,
  vaultMutationsBlocked,
  vaultPutIsOverwrite,
} from "./vault-ops";
export type { VaultClass, VaultGraphEdge, VaultGraphNode, VaultHealthKind } from "./vault-ops";

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
  health: () => jsonFetch<VaultHealth>("/api/vault/health"),
  graph: async (limit = GRAPH_NODE_CAP) =>
    normalizeGraph(await jsonFetch<unknown>(`/api/vault/graph?limit=${encodeURIComponent(String(limit))}`), limit),
};
