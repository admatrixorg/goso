export const NODE_CAP = 40;
export const EDGE_CAP = 80;
export const BODY_CAP = 20_000;
export const SNIPPET_CAP = 80;

export type KgScope = "all" | "posted" | "extracted" | "";

export type KgIndexLite = {
  search?: string;
  fts?: boolean;
  embedding?: string;
  embedding_configured?: boolean;
};

export type KgNodeLite = {
  id: string;
  name: string;
  kind?: string;
  snippet?: string;
  agent_id?: string;
  source?: string;
  inferred?: boolean;
  created_at?: string;
  valid_from?: string;
  valid_until?: string;
  body?: string;
  token?: string;
  secret?: string;
  api_key?: string;
};

export type KgEdgeLite = {
  id: string;
  from_id: string;
  to_id: string;
  from_name?: string;
  to_name?: string;
  rel: string;
  source?: string;
  inferred?: boolean;
  body?: string;
};

export type KgGraphLite = {
  nodes: KgNodeLite[];
  edges: KgEdgeLite[];
  truncated: boolean;
  node_cap: number;
  edge_cap: number;
  total_nodes?: number;
  total_edges?: number;
  inferred_are_not_facts: boolean;
  search?: string;
  fts?: boolean;
  embedding?: string;
  embedding_configured?: boolean;
};

const SECRET_KEYS = new Set([
  "token",
  "secret",
  "password",
  "hmac",
  "hmac_key",
  "code",
  "bot_token",
  "access_token",
  "api_key",
  "private_key",
  "ssh_key",
  "pem",
  "key",
  "body",
]);
const SECRET_VAL =
  /\b(sk-[A-Za-z0-9_-]{8,}|gsk_[A-Za-z0-9]+|xai-[A-Za-z0-9]+|Bearer\s+[A-Za-z0-9._\-+=/]{8,}|BEGIN [A-Z ]*PRIVATE KEY)/i;

export function publicHasSecrets(row: unknown): boolean {
  if (row == null || typeof row !== "object") return false;
  const rec = row as Record<string, unknown>;
  for (const [k, v] of Object.entries(rec)) {
    const key = k.toLowerCase();
    if (SECRET_KEYS.has(key) && typeof v === "string" && v.length > 0) {
      if (key === "body") continue;
      return true;
    }
    if (typeof v === "string" && SECRET_VAL.test(v)) return true;
  }
  return false;
}

export function normalizeScope(scope?: string): KgScope {
  const s = (scope || "").trim().toLowerCase();
  if (s === "posted" || s === "recorded") return "posted";
  if (s === "extracted" || s === "inferred") return "extracted";
  if (s === "all") return "all";
  return "";
}

export function isInferred(row: { inferred?: boolean; source?: string }): boolean {
  if (row.inferred === true) return true;
  const s = (row.source || "").trim().toLowerCase();
  return s === "extracted" || s === "inferred" || s === "heuristic";
}

export function kgSnippet(text?: string, cap = SNIPPET_CAP): string {
  const s = (text || "").replace(/\u0000/g, "").trim();
  if (!s) return "";
  if (SECRET_VAL.test(s)) return "";
  const runes = Array.from(s);
  if (runes.length <= cap) return s;
  return `${runes.slice(0, cap).join("")}…`;
}

export function plainKgBody(raw?: string): string {
  let s = (raw || "").replace(/\u0000/g, "");
  if (SECRET_VAL.test(s)) return "";
  if (s.length > BODY_CAP) s = `${s.slice(0, BODY_CAP)}…`;
  return s;
}

export function isEmbeddingConfigured(idx?: KgIndexLite | null): boolean {
  if (!idx) return false;
  if (idx.embedding_configured === true) return true;
  const e = (idx.embedding || "").trim().toLowerCase();
  return e !== "" && e !== "not_configured";
}

function asNode(raw: unknown): KgNodeLite | null {
  if (!raw || typeof raw !== "object") return null;
  const r = raw as Record<string, unknown>;
  const id = String(r.id || "").trim();
  const name = String(r.name || "").trim();
  if (!id || !name) return null;
  if (publicHasSecrets(raw)) return null;
  const snippet = kgSnippet(String(r.snippet || r.name || ""));
  return {
    id,
    name,
    kind: r.kind ? String(r.kind) : undefined,
    snippet,
    agent_id: r.agent_id ? String(r.agent_id) : undefined,
    source: r.source ? String(r.source) : undefined,
    inferred: Boolean(r.inferred) || isInferred({ source: String(r.source || "") }),
    created_at: r.created_at ? String(r.created_at) : undefined,
    valid_from: r.valid_from ? String(r.valid_from) : undefined,
    valid_until: r.valid_until ? String(r.valid_until) : undefined,
  };
}

function asEdge(raw: unknown): KgEdgeLite | null {
  if (!raw || typeof raw !== "object") return null;
  const r = raw as Record<string, unknown>;
  const id = String(r.id || "").trim();
  const fromId = String(r.from_id || "").trim();
  const toId = String(r.to_id || "").trim();
  const rel = String(r.rel || "").trim();
  if (!fromId || !toId || !rel) return null;
  if (publicHasSecrets(raw)) return null;
  return {
    id: id || `${fromId}:${rel}:${toId}`,
    from_id: fromId,
    to_id: toId,
    from_name: r.from_name ? String(r.from_name) : undefined,
    to_name: r.to_name ? String(r.to_name) : undefined,
    rel,
    source: r.source ? String(r.source) : undefined,
    inferred: Boolean(r.inferred) || isInferred({ source: String(r.source || "") }),
  };
}

export function normalizeGraph(raw: unknown, cap = NODE_CAP): KgGraphLite {
  const limit = cap > 0 ? Math.min(cap, 200) : NODE_CAP;
  const empty: KgGraphLite = {
    nodes: [],
    edges: [],
    truncated: false,
    node_cap: limit,
    edge_cap: limit * 2,
    total_nodes: 0,
    total_edges: 0,
    inferred_are_not_facts: true,
    embedding: "not_configured",
    embedding_configured: false,
  };
  if (!raw || typeof raw !== "object") return empty;
  const rec = raw as Record<string, unknown>;
  const nodesIn = Array.isArray(rec.nodes) ? rec.nodes : [];
  const edgesIn = Array.isArray(rec.edges) ? rec.edges : [];
  const nodes: KgNodeLite[] = [];
  for (const n of nodesIn) {
    const row = asNode(n);
    if (row) nodes.push(row);
  }
  const edges: KgEdgeLite[] = [];
  for (const e of edgesIn) {
    const row = asEdge(e);
    if (row) edges.push(row);
  }
  const nodeCap = Number(rec.node_cap) > 0 ? Number(rec.node_cap) : limit;
  const edgeCap = Number(rec.edge_cap) > 0 ? Number(rec.edge_cap) : nodeCap * 2;
  const truncated = Boolean(rec.truncated) || nodes.length > nodeCap || edges.length > edgeCap;
  return {
    nodes: nodes.slice(0, nodeCap),
    edges: edges.slice(0, edgeCap),
    truncated,
    node_cap: nodeCap,
    edge_cap: edgeCap,
    total_nodes: Number(rec.total_nodes) || nodes.length,
    total_edges: Number(rec.total_edges) || edges.length,
    inferred_are_not_facts: rec.inferred_are_not_facts !== false,
    search: rec.search ? String(rec.search) : undefined,
    fts: Boolean(rec.fts),
    embedding: rec.embedding ? String(rec.embedding) : "not_configured",
    embedding_configured: rec.embedding_configured === true,
  };
}

export function formatWhen(raw?: string, fallback = "—"): string {
  const s = (raw || "").trim();
  if (!s) return fallback;
  const d = new Date(s);
  if (Number.isNaN(d.getTime())) return s;
  return d.toISOString().replace("T", " ").replace(/\.\d+Z$/, " UTC");
}
