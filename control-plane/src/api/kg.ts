import { jsonFetch } from "./client";
import { NODE_CAP, normalizeGraph, type KgGraphLite, type KgIndexLite } from "./kg-ops";

export type KgGraph = KgGraphLite;
export type KgIndex = KgIndexLite;

export type KgRelation = {
  id: string;
  from_id: string;
  to_id: string;
  rel: string;
  body?: string;
  from_name?: string;
  to_name?: string;
  source?: string;
  inferred?: boolean;
};

export type KgExpand = {
  entity?: {
    id: string;
    name: string;
    kind: string;
    body?: string;
    tenant_id?: string;
    agent_id?: string;
    source?: string;
    created_at?: string;
    valid_from?: string;
    valid_until?: string;
  };
  relations?: KgRelation[];
};

export type KgGraphOpts = {
  agent_id: string;
  scope?: string;
  q?: string;
  limit?: number;
};

function graphQuery(opts: KgGraphOpts): string {
  const p = new URLSearchParams();
  p.set("agent_id", opts.agent_id);
  if (opts.scope) p.set("scope", opts.scope);
  if (opts.q) p.set("q", opts.q);
  p.set("limit", String(opts.limit && opts.limit > 0 ? opts.limit : NODE_CAP));
  return p.toString();
}

export const kgApi = {
  graph: async (opts: KgGraphOpts) =>
    normalizeGraph(await jsonFetch<unknown>(`/api/kg/graph?${graphQuery(opts)}`), opts.limit ?? NODE_CAP),
  index: () => jsonFetch<KgIndex>("/api/kg/index"),
  expand: (id: string) => jsonFetch<KgExpand>(`/api/kg/entities/${encodeURIComponent(id)}`),
};

export {
  BODY_CAP,
  EDGE_CAP,
  NODE_CAP,
  classifyKgView,
  formatWhen,
  inferredLabel,
  isEmbeddingConfigured,
  isInferred,
  kgBlocksFetch,
  kgSnippet,
  kgViewPageKind,
  normalizeGraph,
  normalizeScope,
  plainKgBody,
  publicHasSecrets,
} from "./kg-ops";
export type { KgEdgeLite, KgGraphLite, KgIndexLite, KgNodeLite, KgScope, KgViewKind } from "./kg-ops";
