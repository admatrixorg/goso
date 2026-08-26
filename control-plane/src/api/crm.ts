// goso-crm HTTP client — KPI/advisor over HTTP only. Never import goso-crm Go.
// Header X-Org-ID on CRM fetches. No secrets in this module or in displayed errors.

export const CRM_UPSTREAM_DEFAULT = "http://127.0.0.1:8089";
export const CRM_ORG_DEFAULT = "01a01fe5-704c-7375-aa1f-6e50a9d0296d";
export const CRM_PROXY_PREFIX = "/crm-api";
const HEALTH_TIMEOUT_MS = 3000;

export function crmBase(): string {
  const v = import.meta.env.VITE_GOSOCRM_API_URL?.trim();
  if (v) return v.replace(/\/$/, "");
  // Dev: Vite proxy /crm-api → http://127.0.0.1:8089 (CORS-free). Prod: direct default.
  if (import.meta.env.DEV) return CRM_PROXY_PREFIX;
  return CRM_UPSTREAM_DEFAULT;
}

export function crmOrgId(): string {
  const v = import.meta.env.VITE_GOSOCRM_ORG_ID?.trim();
  return v || CRM_ORG_DEFAULT;
}

export function crmUpstream(): string {
  const v = import.meta.env.VITE_GOSOCRM_API_URL?.trim();
  if (v && /^https?:\/\//i.test(v)) return v.replace(/\/$/, "");
  return CRM_UPSTREAM_DEFAULT;
}

function orgHeaders(orgId: string): Record<string, string> {
  const h: Record<string, string> = { Accept: "application/json", "X-Org-ID": orgId };
  const tok = import.meta.env.VITE_GOSOCRM_ORG_TOKEN?.trim();
  if (tok) h["X-Org-Token"] = tok;
  return h;
}

function clip(s: string, n = 240): string {
  const t = s.replace(/Bearer\s+\S+/gi, "Bearer [redacted]");
  return t.length > n ? `${t.slice(0, n)}…` : t;
}

export function asCrmError(e: unknown): string {
  if (typeof DOMException !== "undefined" && e instanceof DOMException && e.name === "AbortError") {
    return "request timed out (~3s)";
  }
  if (e instanceof Error && e.name === "AbortError") return "request timed out (~3s)";
  if (e instanceof Error) return clip(e.message);
  return clip(String(e));
}

async function fetchWithTimeout(url: string, init: RequestInit | undefined, timeoutMs: number): Promise<Response> {
  const ctrl = new AbortController();
  const t = setTimeout(() => ctrl.abort(), timeoutMs);
  try {
    return await fetch(url, { ...init, signal: ctrl.signal });
  } finally {
    clearTimeout(t);
  }
}

async function crmJson<T>(path: string, orgId: string): Promise<T> {
  const res = await fetchWithTimeout(
    `${crmBase()}${path}`,
    { method: "GET", headers: orgHeaders(orgId) },
    HEALTH_TIMEOUT_MS,
  );
  if (!res.ok) {
    const text = await res.text().catch(() => "");
    throw new Error(`${res.status} ${clip(text)}`);
  }
  return (await res.json()) as T;
}

export type CrmHealth = { online: boolean };

/** GET {base}/healthz or /readyz. Fetch error / non-200 / timeout (~3s) → offline. */
export async function crmHealth(): Promise<CrmHealth> {
  const base = crmBase();
  for (const path of ["/healthz", "/readyz"] as const) {
    try {
      const res = await fetchWithTimeout(`${base}${path}`, { method: "GET" }, HEALTH_TIMEOUT_MS);
      if (res.ok) return { online: true };
    } catch {
      // timeout or network — try the other probe
    }
  }
  return { online: false };
}

export type CrmMetrics = {
  orgId?: string;
  from?: string;
  to?: string;
  messagesSent: number;
  messagesReceived: number;
  unreplied: number;
  avgResponseTime: number;
  kpiCompletionRate: number;
  revenueMonth: number;
  sampleDays?: number;
  teamCount?: number;
};

function num(v: unknown): number {
  return typeof v === "number" && Number.isFinite(v) ? v : 0;
}

export async function fetchCrmMetrics(orgId: string): Promise<CrmMetrics> {
  const j = await crmJson<Record<string, unknown>>("/api/crm/metrics", orgId);
  return {
    orgId: typeof j.orgId === "string" ? j.orgId : undefined,
    from: typeof j.from === "string" ? j.from : undefined,
    to: typeof j.to === "string" ? j.to : undefined,
    messagesSent: num(j.messagesSent),
    messagesReceived: num(j.messagesReceived),
    unreplied: num(j.unreplied),
    avgResponseTime: num(j.avgResponseTime),
    kpiCompletionRate: num(j.kpiCompletionRate),
    revenueMonth: num(j.revenueMonth),
    sampleDays: typeof j.sampleDays === "number" ? j.sampleDays : undefined,
    teamCount: typeof j.teamCount === "number" ? j.teamCount : undefined,
  };
}

export type CrmAdvice = {
  kind: string;
  summary: string;
  confidence: number;
  evidenceIds?: string[];
};

export async function fetchCrmAdvisor(orgId: string): Promise<CrmAdvice[]> {
  const j = await crmJson<unknown>("/api/crm/advisor", orgId);
  const rows = Array.isArray(j)
    ? j
    : j && typeof j === "object" && Array.isArray((j as { advice?: unknown }).advice)
      ? (j as { advice: unknown[] }).advice
      : [];
  return rows.map((row) => {
    const r = (row && typeof row === "object" ? row : {}) as Record<string, unknown>;
    return {
      kind: typeof r.kind === "string" ? r.kind : "",
      summary: typeof r.summary === "string" ? r.summary : "",
      confidence: num(r.confidence),
      evidenceIds: Array.isArray(r.evidenceIds) ? r.evidenceIds.filter((x): x is string => typeof x === "string") : undefined,
    };
  });
}
