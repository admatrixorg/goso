import { asList, crmOrgId, crmRequest } from "./crm";

export type AudienceSource = "paste" | "file" | "leadads";
export type CampaignKind = "scan" | "goal" | "care" | "sequence" | "broadcast" | "content";
export type CampaignStatus = "draft" | "scheduled" | "done";

export type Audience = { id: string; orgId: string; name: string; source: AudienceSource; size: number };
export type CampaignItem = { id?: string; campaignId?: string; body: string; sort: number };
export type Campaign = {
  id: string;
  orgId: string;
  name: string;
  kind: CampaignKind;
  status: CampaignStatus;
  channel: string;
  scheduleAt?: string;
  audienceId?: string;
  items: CampaignItem[];
};
export type MarketingOverview = {
  audience: number;
  scan: number;
  goal: number;
  care: number;
  sequence: number;
  broadcast: number;
  content: number;
};

export const MARKETING_TABS: { id: "audience" | CampaignKind; kind: CampaignKind | null }[] = [
  { id: "audience", kind: null },
  { id: "scan", kind: "scan" },
  { id: "goal", kind: "goal" },
  { id: "care", kind: "care" },
  { id: "sequence", kind: "sequence" },
  { id: "broadcast", kind: "broadcast" },
  { id: "content", kind: "content" },
];

function org(orgId?: string): string {
  return (orgId ?? crmOrgId()).trim();
}

function str(v: unknown): string {
  return typeof v === "string" ? v : "";
}

function num(v: unknown): number {
  return typeof v === "number" && Number.isFinite(v) ? v : 0;
}

function sourceOf(v: unknown): AudienceSource {
  return v === "file" || v === "leadads" || v === "paste" ? v : "paste";
}

function kindOf(v: unknown): CampaignKind {
  return v === "scan" || v === "goal" || v === "care" || v === "sequence" || v === "broadcast" || v === "content" ? v : "scan";
}

function statusOf(v: unknown): CampaignStatus {
  return v === "scheduled" || v === "done" || v === "draft" ? v : "draft";
}

function audienceOf(r: Record<string, unknown>): Audience {
  return { id: str(r.id), orgId: str(r.orgId), name: str(r.name), source: sourceOf(r.source), size: num(r.size) };
}

function campaignOf(r: Record<string, unknown>): Campaign {
  const items = Array.isArray(r.items)
    ? r.items
        .filter((x): x is Record<string, unknown> => !!x && typeof x === "object")
        .map((it) => ({ id: str(it.id) || undefined, campaignId: str(it.campaignId) || undefined, body: str(it.body), sort: num(it.sort) }))
    : [];
  return {
    id: str(r.id),
    orgId: str(r.orgId),
    name: str(r.name),
    kind: kindOf(r.kind),
    status: statusOf(r.status),
    channel: str(r.channel),
    scheduleAt: typeof r.scheduleAt === "string" ? r.scheduleAt : undefined,
    audienceId: str(r.audienceId) || undefined,
    items,
  };
}

export const marketingApi = {
  overview: async (orgId?: string) => {
    const j = (await crmRequest<Record<string, unknown>>("/api/marketing/overview", org(orgId))) ?? {};
    return {
      audience: num(j.audience),
      scan: num(j.scan),
      goal: num(j.goal),
      care: num(j.care),
      sequence: num(j.sequence),
      broadcast: num(j.broadcast),
      content: num(j.content),
    } satisfies MarketingOverview;
  },
  listAudiences: async (orgId?: string) => asList<Record<string, unknown>>(await crmRequest("/api/marketing/audiences", org(orgId))).map(audienceOf),
  createAudience: (body: { name: string; source: AudienceSource; size: number }, orgId?: string) =>
    crmRequest<Record<string, unknown>>("/api/marketing/audiences", org(orgId), { method: "POST", body: JSON.stringify(body) }).then(audienceOf),
  listCampaigns: async (orgId?: string) => asList<Record<string, unknown>>(await crmRequest("/api/marketing/campaigns", org(orgId))).map(campaignOf),
  createCampaign: (
    body: { name: string; kind: CampaignKind; status?: CampaignStatus; channel?: string; audienceId?: string; items?: CampaignItem[] },
    orgId?: string,
  ) => crmRequest<Record<string, unknown>>("/api/marketing/campaigns", org(orgId), { method: "POST", body: JSON.stringify(body) }).then(campaignOf),
  patchCampaign: (id: string, body: { name?: string; status?: CampaignStatus; channel?: string; audienceId?: string }, orgId?: string) =>
    crmRequest<Record<string, unknown>>(`/api/marketing/campaigns/${encodeURIComponent(id)}`, org(orgId), {
      method: "PATCH",
      body: JSON.stringify(body),
    }).then(campaignOf),
};
