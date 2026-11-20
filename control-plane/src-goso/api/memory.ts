import { jsonFetch } from "./client";

export type MemoryNote = {
  id: string;
  session_id: string;
  agent_id?: string;
  kind: string;
  snippet?: string;
  body?: string;
  created_at: string;
};

export type MemoryIndex = {
  search: string;
  fts: boolean;
  embedding: string;
  embedding_configured: boolean;
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

export type MemoryListOpts = { session_id?: string; agent_id?: string; kind?: string };

export const memoryApi = {
  list: (opts: MemoryListOpts = {}) => {
    const p = new URLSearchParams();
    if (opts.session_id) p.set("session_id", opts.session_id);
    if (opts.agent_id) p.set("agent_id", opts.agent_id);
    if (opts.kind) p.set("kind", opts.kind);
    const qs = p.toString();
    return jsonFetch<{ memories: MemoryNote[] }>(`/api/memory${qs ? `?${qs}` : ""}`);
  },
  get: (id: string) => jsonFetch<MemoryNote>(`/api/memory/${encodeURIComponent(id)}`),
  create: (body: { session_id: string; body: string; kind?: string }) =>
    jsonFetch<MemoryNote>("/api/memory", { method: "POST", body: JSON.stringify(body) }),
  patch: (id: string, body: { body?: string; kind?: string }) =>
    jsonFetch<MemoryNote>(`/api/memory/${encodeURIComponent(id)}`, { method: "PATCH", body: JSON.stringify(body) }),
  remove: (id: string) => jsonFetch<{ ok: boolean }>(`/api/memory/${encodeURIComponent(id)}`, { method: "DELETE" }),
  index: () => jsonFetch<MemoryIndex>("/api/memory/index"),
  search: async (q: string) => asHits(await jsonFetch<unknown>(`/api/memory/search?q=${encodeURIComponent(q)}`)),
  searchProgressive: async (q: string) => asHits(await jsonFetch<unknown>(`/api/kg/search?q=${encodeURIComponent(q)}`)),
  expand: (id: string) => jsonFetch<KgExpand>(`/api/kg/entities/${encodeURIComponent(id)}`),
};
