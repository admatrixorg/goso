import { useEffect, useMemo, useState, type ReactNode } from "react";
import { crmOrgId } from "../api/crm";
import {
  MARKETING_TABS,
  marketingApi,
  type Audience,
  type AudienceSource,
  type Campaign,
  type CampaignKind,
  type MarketingOverview,
} from "../api/marketing";
import { useI18n, type MsgKey } from "../i18n";
import { Button } from "../ui/Button";
import { Card, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";

const TAB_LABEL: Record<(typeof MARKETING_TABS)[number]["id"], MsgKey> = {
  audience: "mkt.audience",
  scan: "mkt.scan",
  goal: "mkt.goal",
  care: "mkt.care",
  sequence: "mkt.sequence",
  broadcast: "mkt.broadcast",
  content: "mkt.content",
};

const KPI_LABEL: Record<keyof MarketingOverview, MsgKey> = {
  audience: "mkt.kpi.audience",
  scan: "mkt.kpi.scan",
  goal: "mkt.kpi.goal",
  care: "mkt.kpi.care",
  sequence: "mkt.kpi.sequence",
  broadcast: "mkt.kpi.broadcast",
  content: "mkt.kpi.content",
};

export function MarketingPage() {
  const { t } = useI18n();
  const [tab, setTab] = useState<(typeof MARKETING_TABS)[number]["id"]>("audience");
  const [org, setOrg] = useState(crmOrgId);
  const [err, setErr] = useState("");
  const [overview, setOverview] = useState<MarketingOverview | null>(null);
  const [audiences, setAudiences] = useState<Audience[]>([]);
  const [campaigns, setCampaigns] = useState<Campaign[]>([]);

  const [audName, setAudName] = useState("");
  const [audSource, setAudSource] = useState<AudienceSource>("paste");
  const [audSize, setAudSize] = useState("0");
  const [campName, setCampName] = useState("");
  const [campChannel, setCampChannel] = useState("");
  const [campAudience, setCampAudience] = useState("");

  const kind = MARKETING_TABS.find((x) => x.id === tab)?.kind ?? null;
  const filtered = useMemo(() => (kind ? campaigns.filter((c) => c.kind === kind) : []), [campaigns, kind]);

  async function load() {
    const id = org.trim() || crmOrgId();
    setErr("");
    try {
      const [ov, aud, camp] = await Promise.all([
        marketingApi.overview(id),
        marketingApi.listAudiences(id),
        marketingApi.listCampaigns(id),
      ]);
      setOverview(ov);
      setAudiences(aud);
      setCampaigns(camp);
    } catch (e) {
      setErr(String(e));
    }
  }

  useEffect(() => {
    void load();
  }, [org]);

  async function run(fn: () => Promise<unknown>) {
    try {
      setErr("");
      await fn();
      await load();
    } catch (e) {
      setErr(String(e));
    }
  }

  const kpis: { key: keyof MarketingOverview; value: number }[] = overview
    ? (Object.keys(KPI_LABEL) as (keyof MarketingOverview)[]).map((key) => ({ key, value: overview[key] }))
    : [];

  return (
    <div className="z-split-stack">
      <div className="z-split-rail" style={{ width: 210, background: "var(--card)", borderRight: "1px solid var(--border)", padding: "14px 10px", display: "flex", flexDirection: "column", gap: 4 }}>
        <div style={{ display: "flex", gap: 8, alignItems: "center", fontWeight: 700, fontSize: 15, padding: "0 8px 10px" }}>{t("mkt.title")}</div>
        {MARKETING_TABS.map((m) => {
          const on = tab === m.id;
          const count = overview ? overview[m.id] : undefined;
          return (
            <button
              key={m.id}
              type="button"
              onClick={() => setTab(m.id)}
              style={{
                border: "none",
                borderRadius: 8,
                padding: "7px 12px",
                fontSize: 13,
                background: on ? "var(--accent-soft)" : "transparent",
                color: on ? "var(--accent)" : "var(--text-2)",
                fontWeight: on ? 600 : 400,
                textAlign: "left",
                display: "flex",
                gap: 8,
                alignItems: "center",
              }}
            >
              <span style={{ flex: 1 }}>{t(TAB_LABEL[m.id])}</span>
              {count != null ? <span style={{ fontSize: 11, color: "var(--text-3)", fontVariantNumeric: "tabular-nums" }}>{count}</span> : null}
            </button>
          );
        })}
      </div>
      <div style={{ flex: 1, overflowY: "auto", padding: "14px 22px", display: "flex", flexDirection: "column", gap: 12 }}>
        <div style={{ display: "flex", gap: 8, alignItems: "center", flexWrap: "wrap" }}>
          <span style={{ fontSize: 12, color: "var(--text-3)" }}>{t("common.org")}</span>
          <input className="z-field" style={{ minWidth: 0, flex: 1 }} value={org} onChange={(e) => setOrg(e.target.value)} aria-label="CRM org id" />
          <Button icon="refresh" iconGesture onClick={() => void load()}>
            {t("common.refresh")}
          </Button>
        </div>
        {err ? <p style={{ color: "var(--red)", fontSize: 12.5, margin: 0 }}>{err}</p> : null}
        {kpis.length ? (
          <div style={{ display: "flex", gap: 10, flexWrap: "wrap" }}>
            {kpis.map((k) => (
              <Card key={k.key} style={{ flex: "1 1 120px", minWidth: 0, padding: "11px 14px" }}>
                <div style={{ fontSize: 20, fontWeight: 700, fontVariantNumeric: "tabular-nums" }}>{k.value}</div>
                <div style={{ fontSize: 11, color: "var(--text-3)" }}>{t(KPI_LABEL[k.key])}</div>
              </Card>
            ))}
          </div>
        ) : null}

        {tab === "audience" ? (
          <>
            <div style={{ display: "flex", gap: 10, alignItems: "flex-start" }}>
              <div style={{ flex: 1 }}>
                <div style={{ fontSize: 17, fontWeight: 700 }}>{t("mkt.audience")}</div>
                <div style={{ fontSize: 12, color: "var(--text-3)", maxWidth: 640 }}>{t("mkt.audience.desc")}</div>
              </div>
            </div>
            <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
              <input className="z-field" placeholder={t("common.name")} value={audName} onChange={(e) => setAudName(e.target.value)} />
              <select className="z-field" value={audSource} onChange={(e) => setAudSource(e.target.value as AudienceSource)}>
                <option value="paste">{t("mkt.source.paste")}</option>
                <option value="file">{t("mkt.source.file")}</option>
                <option value="leadads">{t("mkt.source.leadads")}</option>
              </select>
              <input className="z-field" type="number" min={0} placeholder={t("mkt.size")} value={audSize} onChange={(e) => setAudSize(e.target.value)} style={{ width: 100 }} />
              <Button
                variant="primary"
                icon="plus"
                onClick={() =>
                  void run(async () => {
                    if (!audName.trim()) return;
                    await marketingApi.createAudience(
                      { name: audName.trim(), source: audSource, size: Number.parseInt(audSize, 10) || 0 },
                      org,
                    );
                    setAudName("");
                    setAudSize("0");
                  })
                }
              >
                {t("mkt.createAudience")}
              </Button>
            </div>
            <Card>
              <TableScroll>
              <Head>
                <span style={{ flex: 2 }}>{t("mkt.col.name")}</span>
                <span style={{ flex: 1 }}>{t("mkt.col.source")}</span>
                <span style={{ flex: 0.8, textAlign: "right" }}>{t("mkt.col.size")}</span>
              </Head>
              {audiences.map((a) => (
                <Head key={a.id} body>
                  <span style={{ flex: 2, fontWeight: 600 }}>{a.name}</span>
                  <span style={{ flex: 1, color: "var(--text-2)" }}>{a.source}</span>
                  <span style={{ flex: 0.8, textAlign: "right", fontVariantNumeric: "tabular-nums" }}>{a.size}</span>
                </Head>
              ))}
              {audiences.length === 0 ? <EmptyState>{t("mkt.emptyAudience")}</EmptyState> : null}
              </TableScroll>
            </Card>
          </>
        ) : (
          <>
            <div>
              <div style={{ fontSize: 17, fontWeight: 700 }}>{t(TAB_LABEL[tab])}</div>
              <div style={{ fontSize: 12, color: "var(--text-3)", maxWidth: 640 }}>{t("mkt.campaign.desc", { kind: kind ?? tab })}</div>
            </div>
            <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
              <input className="z-field" placeholder={t("common.name")} value={campName} onChange={(e) => setCampName(e.target.value)} />
              <input className="z-field" placeholder={t("mkt.channel")} value={campChannel} onChange={(e) => setCampChannel(e.target.value)} />
              <select className="z-field" value={campAudience} onChange={(e) => setCampAudience(e.target.value)}>
                <option value="">{t("mkt.audienceId")}</option>
                {audiences.map((a) => (
                  <option key={a.id} value={a.id}>
                    {a.name}
                  </option>
                ))}
              </select>
              <Button
                variant="primary"
                icon="plus"
                onClick={() =>
                  void run(async () => {
                    if (!campName.trim() || !kind) return;
                    await marketingApi.createCampaign(
                      {
                        name: campName.trim(),
                        kind: kind as CampaignKind,
                        status: "draft",
                        channel: campChannel.trim(),
                        audienceId: campAudience || undefined,
                        items: [],
                      },
                      org,
                    );
                    setCampName("");
                    setCampChannel("");
                  })
                }
              >
                {t("mkt.createCampaign")}
              </Button>
            </div>
            <Card>
              <TableScroll>
              <Head>
                <span style={{ flex: 2 }}>{t("mkt.col.name")}</span>
                <span style={{ flex: 0.8 }}>{t("mkt.col.kind")}</span>
                <span style={{ flex: 0.9 }}>{t("mkt.col.status")}</span>
                <span style={{ flex: 1 }}>{t("mkt.col.channel")}</span>
                <span style={{ width: 110 }} />
              </Head>
              {filtered.map((c) => (
                <Head key={c.id} body>
                  <span style={{ flex: 2, fontWeight: 600 }}>{c.name}</span>
                  <span style={{ flex: 0.8, color: "var(--text-2)" }}>{c.kind}</span>
                  <span style={{ flex: 0.9 }}>{statusLabel(c.status, t)}</span>
                  <span style={{ flex: 1, color: "var(--text-3)" }}>{c.channel || "—"}</span>
                  <span style={{ width: 110, textAlign: "right" }}>
                    {c.status !== "done" ? (
                      <Button variant="quiet" style={{ padding: "4px 8px" }} onClick={() => void run(() => marketingApi.patchCampaign(c.id, { status: "done" }, org))}>
                        {t("mkt.patchDone")}
                      </Button>
                    ) : null}
                  </span>
                </Head>
              ))}
              {filtered.length === 0 ? <EmptyState>{t("mkt.emptyCampaign")}</EmptyState> : null}
              </TableScroll>
            </Card>
          </>
        )}
      </div>
    </div>
  );
}

function statusLabel(status: Campaign["status"], t: (k: MsgKey) => string): string {
  if (status === "scheduled") return t("mkt.status.scheduled");
  if (status === "done") return t("mkt.status.done");
  return t("mkt.status.draft");
}

function Head({ children, body }: { children: ReactNode; body?: boolean }) {
  return (
    <div
      style={{
        display: "flex",
        alignItems: "center",
        padding: body ? "11px 16px" : "8px 16px",
        fontSize: body ? 12.5 : 10,
        fontWeight: body ? 400 : 600,
        letterSpacing: body ? undefined : ".4px",
        color: body ? undefined : "var(--text-3)",
        borderBottom: "1px solid var(--border-soft)",
      }}
    >
      {children}
    </div>
  );
}
