export const LIST_CAP = 200;
export const BODY_CAP = 20_000;
export const GRAPH_NODE_CAP = 40;
export const GRAPH_EDGE_CAP = 80;

export type VaultDocLite = {
  id: string;
  title?: string;
  path?: string;
  body?: string;
  sha256?: string;
  mtime?: string;
};

export type VaultLinkLite = { from_id: string; to_id?: string; raw: string };

export type VaultClass = { type: string; agent: string; team: string };

export type VaultHealthLite = {
  docs?: number;
  disk_files?: number;
  missing_on_disk?: number;
  unindexed?: number;
  hash_mismatch?: number;
  stale?: boolean;
};

export type VaultGraphNode = { id: string; title: string; path: string };
export type VaultGraphEdge = { from_id: string; to_id?: string; raw: string };
export type VaultGraphLite = {
  nodes: VaultGraphNode[];
  edges: VaultGraphEdge[];
  truncated: boolean;
  node_cap: number;
};

function token(raw: string): string {
  return (raw || "").trim().replace(/[<>]/g, "").slice(0, 64);
}

function slashPath(path: string): string {
  return (path || "").replace(/\\/g, "/").replace(/^\/+/, "");
}

/** Leading YAML-like `---` block. Only type/agent/team keys are read. */
export function parseVaultFrontmatter(body?: string): Partial<VaultClass> {
  const src = (body || "").replace(/^\uFEFF/, "");
  if (!src.startsWith("---")) return {};
  const rest = src.slice(3);
  const nl = rest.startsWith("\n") || rest.startsWith("\r\n") ? rest.replace(/^\r?\n/, "") : rest;
  const end = nl.search(/\r?\n---\s*(?:\r?\n|$)/);
  if (end < 0) return {};
  const block = nl.slice(0, end);
  const out: Partial<VaultClass> = {};
  for (const line of block.split(/\r?\n/)) {
    const m = line.match(/^(type|agent|team)\s*:\s*(.*)$/i);
    if (!m) continue;
    let v = m[2].trim();
    if ((v.startsWith('"') && v.endsWith('"')) || (v.startsWith("'") && v.endsWith("'"))) {
      v = v.slice(1, -1);
    }
    const key = m[1].toLowerCase() as keyof VaultClass;
    const t = token(v);
    if (t) out[key] = t;
  }
  return out;
}

function typeFromPath(path: string): string {
  const p = slashPath(path);
  const parts = p.split("/").filter(Boolean);
  const base = (parts[parts.length - 1] || "").toLowerCase();
  if (base === "team.md") return "team";
  if (parts[0] === "agents" && parts.length > 1) return "agent";
  if (parts[0] === "teams" && parts.length > 1) return "team";
  if (base.endsWith(".txt")) return "text";
  if (base.endsWith(".md")) return "markdown";
  return "note";
}

function agentFromPath(path: string): string {
  const parts = slashPath(path).split("/").filter(Boolean);
  if (parts[0] === "agents" && parts[1]) return token(parts[1]);
  return "";
}

function teamFromPath(path: string): string {
  const p = slashPath(path);
  const parts = p.split("/").filter(Boolean);
  const base = (parts[parts.length - 1] || "").toLowerCase();
  if (base === "team.md") return "team";
  if (parts[0] === "teams" && parts[1]) return token(parts[1]);
  return "";
}

export function classifyDoc(doc: VaultDocLite): VaultClass {
  const fm = parseVaultFrontmatter(doc.body);
  return {
    type: fm.type || typeFromPath(doc.path || ""),
    agent: fm.agent || agentFromPath(doc.path || ""),
    team: fm.team || teamFromPath(doc.path || ""),
  };
}

export function filterVaultDocs<T extends VaultDocLite>(
  docs: T[],
  opts: { query?: string; type?: string; agent?: string; team?: string } = {},
): T[] {
  const q = (opts.query || "").trim().toLowerCase();
  const type = (opts.type || "").trim().toLowerCase();
  const agent = (opts.agent || "").trim().toLowerCase();
  const team = (opts.team || "").trim().toLowerCase();
  return docs.filter((d) => {
    const c = classifyDoc(d);
    if (type && c.type.toLowerCase() !== type) return false;
    if (agent && c.agent.toLowerCase() !== agent) return false;
    if (team && c.team.toLowerCase() !== team) return false;
    if (!q) return true;
    const hay = `${d.title || ""} ${d.path || ""} ${d.id} ${c.type} ${c.agent} ${c.team}`.toLowerCase();
    return hay.includes(q);
  });
}

export function uniqueField(docs: VaultDocLite[], field: keyof VaultClass): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const d of docs) {
    const v = classifyDoc(d)[field].trim();
    if (!v) continue;
    const key = v.toLowerCase();
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(v);
  }
  return out.sort((a, b) => a.localeCompare(b));
}

/** Untrusted vault body is plain text. Never HTML. React text nodes escape it. */
export function plainVaultBody(raw?: string): string {
  let s = (raw || "").replace(/\u0000/g, "");
  if (s.length > BODY_CAP) s = `${s.slice(0, BODY_CAP)}…`;
  return s;
}

export function isStaleHealth(h?: VaultHealthLite | null): boolean {
  if (!h) return false;
  if (h.stale === true) return true;
  return (h.missing_on_disk || 0) + (h.unindexed || 0) + (h.hash_mismatch || 0) > 0;
}

export function capRows<T>(rows: T[], cap = LIST_CAP): { rows: T[]; truncated: boolean } {
  if (rows.length <= cap) return { rows, truncated: false };
  return { rows: rows.slice(0, cap), truncated: true };
}

function titleOf(docs: VaultDocLite[], id: string, fallback: string): { title: string; path: string } {
  const d = docs.find((x) => x.id === id);
  return { title: (d?.title || "").trim() || fallback, path: d?.path || "" };
}

export function boundNeighborhood(
  selected: VaultDocLite | null,
  inbound: VaultLinkLite[],
  outbound: VaultLinkLite[],
  docs: VaultDocLite[],
  cap = GRAPH_NODE_CAP,
): VaultGraphLite {
  const nodeCap = cap > 0 ? cap : GRAPH_NODE_CAP;
  const nodes: VaultGraphNode[] = [];
  const seen = new Set<string>();
  let truncated = false;

  const add = (id: string, title: string, path: string) => {
    if (!id || seen.has(id)) return;
    if (nodes.length >= nodeCap) {
      truncated = true;
      return;
    }
    seen.add(id);
    nodes.push({ id, title, path });
  };

  if (selected?.id) {
    add(selected.id, (selected.title || "").trim() || selected.id, selected.path || "");
  }
  for (const l of outbound) {
    if (l.to_id) {
      const t = titleOf(docs, l.to_id, l.raw || l.to_id);
      add(l.to_id, t.title, t.path);
    } else if (l.raw) {
      add(`raw:${l.raw}`, l.raw, "");
    }
  }
  for (const l of inbound) {
    const t = titleOf(docs, l.from_id, l.from_id);
    add(l.from_id, t.title, t.path);
  }

  const edges: VaultGraphEdge[] = [];
  for (const l of [...outbound, ...inbound]) {
    if (edges.length >= GRAPH_EDGE_CAP) {
      truncated = true;
      break;
    }
    edges.push({ from_id: l.from_id, to_id: l.to_id, raw: l.raw });
  }
  return { nodes, edges, truncated, node_cap: nodeCap };
}

export function normalizeGraph(raw: unknown, cap = GRAPH_NODE_CAP): VaultGraphLite {
  const nodeCap = cap > 0 ? cap : GRAPH_NODE_CAP;
  const empty: VaultGraphLite = { nodes: [], edges: [], truncated: false, node_cap: nodeCap };
  if (!raw || typeof raw !== "object") return empty;
  const j = raw as {
    nodes?: unknown;
    edges?: unknown;
    truncated?: unknown;
    node_cap?: unknown;
  };
  const nodesIn = Array.isArray(j.nodes) ? j.nodes : [];
  const edgesIn = Array.isArray(j.edges) ? j.edges : [];
  const nodes: VaultGraphNode[] = [];
  const seen = new Set<string>();
  let truncated = j.truncated === true;
  for (const n of nodesIn) {
    if (!n || typeof n !== "object") continue;
    const row = n as { id?: unknown; title?: unknown; path?: unknown };
    const id = typeof row.id === "string" ? row.id : "";
    if (!id || seen.has(id)) continue;
    if (nodes.length >= nodeCap) {
      truncated = true;
      break;
    }
    seen.add(id);
    nodes.push({
      id,
      title: typeof row.title === "string" && row.title.trim() ? row.title : id,
      path: typeof row.path === "string" ? row.path : "",
    });
  }
  const edges: VaultGraphEdge[] = [];
  for (const e of edgesIn) {
    if (!e || typeof e !== "object") continue;
    if (edges.length >= GRAPH_EDGE_CAP) {
      truncated = true;
      break;
    }
    const row = e as { from_id?: unknown; to_id?: unknown; raw?: unknown };
    const from = typeof row.from_id === "string" ? row.from_id : "";
    if (!from) continue;
    edges.push({
      from_id: from,
      to_id: typeof row.to_id === "string" ? row.to_id : "",
      raw: typeof row.raw === "string" ? row.raw : "",
    });
  }
  const parsedCap = typeof j.node_cap === "number" && j.node_cap > 0 ? j.node_cap : nodeCap;
  return { nodes, edges, truncated, node_cap: parsedCap };
}

export function shortHash(sha?: string): string {
  const s = (sha || "").trim();
  if (!s) return "—";
  return s.length > 12 ? s.slice(0, 12) : s;
}

export function formatMtime(mtime?: string): string {
  const s = (mtime || "").trim();
  if (!s) return "—";
  const d = new Date(s);
  if (Number.isNaN(d.getTime())) return s;
  return d.toISOString().replace("T", " ").replace(/\.\d+Z$/, " UTC");
}
