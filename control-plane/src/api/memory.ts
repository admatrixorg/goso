import { jsonFetch } from "./client";

export type MemoryNote = {
  id: string;
  session_id: string;
  kind: string;
  body: string;
  created_at: string;
};
export type MemoryHit = {
  id: string;
  session_id?: string;
  kind?: string;
  name?: string;
  snippet: string;
  tier?: string;
};

export type KgRelation = {
  id: string;
  from_id: string;
  to_id: string;
  rel: string;
  body?: string;
  from_name?: string;
  to_name?: string;
};

export type KgExpand = {
  entity?: {
    id: string;
    name: string;
    kind: string;
    body?: string;
    tenant_id?: string;
  };
  relations?: KgRelation[];
};

function asHits(j: unknown): MemoryHit[] {
  if (Array.isArray(j)) return j as MemoryHit[];
  if (j && typeof j === "object") {
    const inner = (j as { hits?: unknown }).hits;
    if (Array.isArray(inner)) return inner as MemoryHit[];
  }
  return [];
}

export const memoryApi = {
  list: (sessionId: string) =>
    jsonFetch<{ memories: MemoryNote[] }>(`/api/memory?session_id=${encodeURIComponent(sessionId)}`),
  create: (body: { session_id: string; body: string; kind?: string }) =>
    jsonFetch<MemoryNote>("/api/memory", { method: "POST", body: JSON.stringify(body) }),
  search: async (q: string) => asHits(await jsonFetch<unknown>(`/api/memory/search?q=${encodeURIComponent(q)}`)),
  searchProgressive: async (q: string) => asHits(await jsonFetch<unknown>(`/api/kg/search?q=${encodeURIComponent(q)}`)),
  expand: (id: string) => jsonFetch<KgExpand>(`/api/kg/entities/${encodeURIComponent(id)}`),
};
