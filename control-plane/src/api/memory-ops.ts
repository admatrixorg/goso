export const LIST_CAP = 200;
export const BODY_CAP = 20_000;
export const SNIPPET_CAP = 80;

export type MemoryLane = "episodic" | "durable";

export type MemoryRow = {
  id: string;
  session_id: string;
  agent_id?: string;
  kind: string;
  snippet?: string;
  body?: string;
};

export type MemoryIndexLite = {
  search?: string;
  fts?: boolean;
  embedding?: string;
  embedding_configured?: boolean;
};

export function normalizeKind(kind?: string): string {
  const k = (kind || "").trim().toLowerCase();
  if (!k || k === "episodic") return "episodic";
  if (k === "document" || k === "durable") return "durable";
  return k;
}

export function memoryLane(kind?: string): MemoryLane {
  const k = normalizeKind(kind);
  if (k === "episodic" || k === "message") return "episodic";
  return "durable";
}

export function memorySnippet(text?: string, cap = SNIPPET_CAP): string {
  const s = (text || "").replace(/\u0000/g, "").trim();
  if (!s) return "";
  const runes = Array.from(s);
  if (runes.length <= cap) return s;
  return `${runes.slice(0, cap).join("")}…`;
}

/** Untrusted note body is plain text. Never HTML. React text nodes escape it. */
export function plainMemoryBody(raw?: string): string {
  let s = (raw || "").replace(/\u0000/g, "");
  if (s.length > BODY_CAP) s = `${s.slice(0, BODY_CAP)}…`;
  return s;
}

export function listTargetName(row: { snippet?: string; body?: string; id?: string; kind?: string }): string {
  const snip = memorySnippet(row.snippet || row.body || "", 48);
  if (snip) return snip;
  return (row.id || "").trim() || (row.kind || "memory");
}

export function filterMemories<T extends MemoryRow>(
  rows: T[],
  opts: { query?: string; agent?: string; session?: string; lane?: string } = {},
): T[] {
  const q = (opts.query || "").trim().toLowerCase();
  const agent = (opts.agent || "").trim().toLowerCase();
  const session = (opts.session || "").trim().toLowerCase();
  const lane = (opts.lane || "").trim().toLowerCase();
  return rows.filter((r) => {
    if (agent && (r.agent_id || "").toLowerCase() !== agent) return false;
    if (session && (r.session_id || "").toLowerCase() !== session) return false;
    if (lane === "episodic" || lane === "durable") {
      if (memoryLane(r.kind) !== lane) return false;
    }
    if (!q) return true;
    const hay = `${r.snippet || ""} ${r.kind} ${r.id} ${r.session_id} ${r.agent_id || ""}`.toLowerCase();
    return hay.includes(q);
  });
}

export function hasBothLanes(rows: MemoryRow[]): boolean {
  let epi = false;
  let dur = false;
  for (const r of rows) {
    if (memoryLane(r.kind) === "episodic") epi = true;
    else dur = true;
    if (epi && dur) return true;
  }
  return false;
}

export function isEmbeddingConfigured(idx?: MemoryIndexLite | null): boolean {
  if (!idx) return false;
  if (idx.embedding_configured === true) return true;
  const e = (idx.embedding || "").trim().toLowerCase();
  return e !== "" && e !== "not_configured";
}

export function capRows<T>(rows: T[], cap = LIST_CAP): { rows: T[]; truncated: boolean } {
  if (rows.length <= cap) return { rows, truncated: false };
  return { rows: rows.slice(0, cap), truncated: true };
}
