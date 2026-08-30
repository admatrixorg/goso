import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { crmHealth, crmOrgId } from "../api/crm";
import {
  MARKETING_TABS,
  marketingApi,
  type Audience,
  type AudienceSource,
  type Campaign,
  type CampaignKind,
  type MarketingOverview,
} from "../api/marketing";
import { audienceSourceNote } from "../api/marketing-ops";
import { classifyPageState, formatStaleAt, inventoryBlocksMutation, isFilteredEmpty, isPermissionError, listMetaCount } from "../api/page-state";
import { useI18n, type MsgKey } from "../i18n";
import { Button } from "../ui/Button";
import { Card, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { PageChrome } from "../ui/PageChrome";
import { PageStatus } from "../ui/PageStatus";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

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

const TAB_ROUTE: Record<(typeof MARKETING_TABS)[number]["id"], string> = {
  audience: "/api/marketing/audiences",
  scan: "/api/marketing/campaigns",
  goal: "/api/marketing/campaigns",
  care: "/api/marketing/campaigns",
  sequence: "/api/marketing/campaigns",
  broadcast: "/api/marketing/campaigns",
  content: "/api/marketing/campaigns",
};

export function MarketingPage() {
  const { t, locale } = useI18n();
  const [tab, setTab] = useState<(typeof MARKETING_TABS)[number]["id"]>("audience");
  const [org, setOrg] = useState(crmOrgId);
  const [orgDraft, setOrgDraft] = useState(crmOrgId);
  const [err, setErr] = useState<unknown>(null);
  const [formErr, setFormErr] = useState("");
  const [overview, setOverview] = useState<MarketingOverview | null>(null);
  const [audiences, setAudiences] = useState<Audience[]>([]);
  const [campaigns, setCampaigns] = useState<Campaign[]>([]);
  const [loading, setLoading] = useState(true);
  const [loaded, setLoaded] = useState(false);
  const [loadedAt, setLoadedAt] = useState<string | null>(null);
  const [crmOnline, setCrmOnline] = useState<boolean | null>(null);

  const [audName, setAudName] = useState("");
  const [audSource, setAudSource] = useState<AudienceSource>("paste");
  const [audSize, setAudSize] = useState("0");
  const [campName, setCampName] = useState("");
  const [campChannel, setCampChannel] = useState("");
  const [campAudience, setCampAudience] = useState("");
  const [createOpen, setCreateOpen] = useState(false);
  const loadSeq = useRef(0);

  const kind = MARKETING_TABS.find((x) => x.id === tab)?.kind ?? null;
  const filtered = useMemo(() => (kind ? campaigns.filter((c) => c.kind === kind) : []), [campaigns, kind]);
  const inventoryCount = tab === "audience" ? audiences.length : campaigns.length;
  const state = classifyPageState({
    loading,
    loaded,
    error: err,
    itemCount: inventoryCount,
    keepStale: loaded && inventoryCount > 0,
  });
  const blocked = inventoryBlocksMutation(state.kind) || crmOnline === false;
  const formVisible = !blocked && createOpen;
  const metaN = listMetaCount(state.kind, tab === "audience" ? audiences.length : filtered.length);
  const sourceNote = audienceSourceNote(audSource);
  const kindEmpty = tab !== "audience" && isFilteredEmpty(state, campaigns.length, filtered.length);

  function commitOrg() {
    const next = orgDraft.trim() || crmOrgId();
    setOrgDraft(next);
    setOrg(next);
  }

  async function load() {
    const seq = ++loadSeq.current;
    const id = org.trim() || crmOrgId();
    setLoading(true);
    setFormErr("");
    const health = await crmHealth();
    if (seq !== loadSeq.current) return;
    setCrmOnline(health.online);
    if (!health.online) {
      setErr(new Error("offline"));
      setLoading(false);
      return;
    }
    try {
      const [ov, aud, camp] = await Promise.all([
        marketingApi.overview(id),
        marketingApi.listAudiences(id),
        marketingApi.listCampaigns(id),
      ]);
      if (seq !== loadSeq.current) return;
      setOverview(ov);
      setAudiences(aud);
      setCampaigns(camp);
      setLoaded(true);
      setLoadedAt(new Date().toISOString());
      setErr(null);
    } catch (e) {
      if (seq !== loadSeq.current) return;
      setErr(e);
    } finally {
      if (seq === loadSeq.current) setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, [org]);

  async function run(fn: () => Promise<unknown>) {
    if (blocked) return;
    try {
      setFormErr("");
      await fn();
      await load();
    } catch (e) {
      setFormErr(formatPublicError(e));
    }
  }

  async function createAudience() {
    if (blocked) return;
    if (!audName.trim()) {
      setFormErr(t("mkt.needName"));
      return;
    }
    await run(async () => {
      await marketingApi.createAudience(
        { name: audName.trim(), source: audSource, size: Number.parseInt(audSize, 10) || 0 },
        org,
      );
      setAudName("");
      setAudSize("0");
      setCreateOpen(false);
    });
  }

  async function createCampaign() {
    if (blocked || !kind) return;
    if (!campName.trim()) {
      setFormErr(t("mkt.needCampaign"));
      return;
    }
    await run(async () => {
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
      setCreateOpen(false);
    });
  }

  const kpis: { key: keyof MarketingOverview; value: number }[] =
    overview && state.kind !== "error" && state.kind !== "permission"
      ? (Object.keys(KPI_LABEL) as (keyof MarketingOverview)[]).map((key) => ({ key, value: overview[key] }))
      : [];

  const statusKind = state.kind;
  const errText =
    statusKind === "stale"
      ? ""
      : err && String(err).includes("offline")
        ? t("mkt.offline")
        : err && isPermissionError(err)
          ? t("crm.permission")
          : err
            ? formatPublicError(err)
            : "";

  return (
    <PageChrome
      icon="mega"
      title={t("mkt.title")}
      description={t("mkt.desc")}
      primary={
        <Button
          variant="primary"
          icon="plus"
          disabled={blocked}
          onClick={() => {
            if (blocked) return;
            setCreateOpen(true);
            setFormErr("");
          }}
        >
          {tab === "audience" ? t("mkt.createAudience") : t("mkt.createCampaign")}
        </Button>
      }
      refresh={
        <Button icon="refresh" iconGesture onClick={() => void load()}>
          {t("common.refresh")}
        </Button>
      }
      filters={
        <>
          <span style={{ fontSize: 12, color: "var(--text-3)" }}>{t("mkt.orgBound")}</span>
          <span style={{ fontSize: 12, color: "var(--text-3)" }}>{t("common.org")}</span>
          <input
            className="z-field"
            style={{ minWidth: 0, flex: 1 }}
            value={orgDraft}
            onChange={(e) => setOrgDraft(e.target.value)}
            onBlur={commitOrg}
            onKeyDown={(e) => {
              if (e.key === "Enter") commitOrg();
            }}
            aria-label={t("common.org")}
          />
        </>
      }
    >
      <div style={{ display: "flex", gap: 6, flexWrap: "wrap" }}>
        {MARKETING_TABS.map((m) => {
          const on = tab === m.id;
          const count = overview && !blocked ? overview[m.id] : undefined;
          return (
            <button
              key={m.id}
              type="button"
              onClick={() => {
                setTab(m.id);
                setCreateOpen(false);
                setFormErr("");
              }}
              style={{
                border: "1px solid var(--border)",
                borderRadius: 8,
                padding: "6px 10px",
                fontSize: 13,
                background: on ? "var(--accent-soft)" : "var(--card)",
                color: on ? "var(--accent)" : "var(--text-2)",
                fontWeight: on ? 600 : 400,
              }}
            >
              {t(TAB_LABEL[m.id])}
              {count != null ? <span style={{ marginLeft: 8, fontSize: 11, color: "var(--text-3)", fontVariantNumeric: "tabular-nums" }}>{count}</span> : null}
            </button>
          );
        })}
      </div>
      <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("mkt.tab.backed", { route: TAB_ROUTE[tab] })}</p>
      <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>
        {crmOnline === true ? t("crm.online") : crmOnline === false ? t("crm.offline") : t("crm.checking")}
      </p>
      <PageStatus
        kind={statusKind === "error" && err && isPermissionError(err) ? "permission" : statusKind}
        errorText={errText}
        staleAt={formatStaleAt(loadedAt, locale)}
        onReload={() => void load()}
      />
      {formErr ? <StatusLine kind="error">{formErr}</StatusLine> : null}

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
          <div>
            <div style={{ fontSize: 17, fontWeight: 700 }}>
              {t("mkt.audience")}
              <span style={{ marginLeft: 8, fontSize: 12, color: "var(--text-3)", fontWeight: 400 }}>{metaN == null ? "—" : t("mkt.kpi.audience") + ` · ${metaN}`}</span>
            </div>
            <div style={{ fontSize: 12, color: "var(--text-3)", maxWidth: 640 }}>{t("mkt.audience.desc")}</div>
          </div>
          {formVisible ? (
            <div style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "flex-end" }}>
              <input className="z-field" placeholder={t("common.name")} value={audName} onChange={(e) => setAudName(e.target.value)} />
              <select className="z-field" value={audSource} onChange={(e) => setAudSource(e.target.value as AudienceSource)}>
                <option value="paste">{t("mkt.source.paste")}</option>
                <option value="file">{t("mkt.source.file")}</option>
                <option value="leadads">{t("mkt.source.leadads")}</option>
              </select>
              <input className="z-field" type="number" min={0} placeholder={t("mkt.size")} value={audSize} onChange={(e) => setAudSize(e.target.value)} style={{ width: 100 }} />
              <Button variant="primary" icon="plus" onClick={() => void createAudience()}>
                {t("mkt.createAudience")}
              </Button>
              <Button variant="quiet" onClick={() => setCreateOpen(false)}>
                {t("common.cancel")}
              </Button>
              <p style={{ margin: 0, width: "100%", fontSize: 12, color: "var(--text-3)" }}>
                {sourceNote === "file" ? t("mkt.source.file.note") : sourceNote === "leadads" ? t("mkt.source.leadads.note") : t("mkt.source.paste.note")}
              </p>
            </div>
          ) : null}
          <Card>
            <TableScroll>
            <Head>
              <span style={{ flex: 2 }}>{t("mkt.col.name")}</span>
              <span style={{ flex: 1 }}>{t("mkt.col.source")}</span>
              <span style={{ flex: 0.8, textAlign: "right" }}>{t("mkt.col.size")}</span>
            </Head>
            {state.showItems
              ? audiences.map((a) => (
                  <Head key={a.id} body>
                    <span style={{ flex: 2, fontWeight: 600 }}>{a.name}</span>
                    <span style={{ flex: 1, color: "var(--text-2)" }}>{t(sourceKey(a.source))}</span>
                    <span style={{ flex: 0.8, textAlign: "right", fontVariantNumeric: "tabular-nums" }}>{a.size}</span>
                  </Head>
                ))
              : null}
            {state.showEmpty ? <EmptyState>{t("mkt.emptyAudience")}</EmptyState> : null}
            </TableScroll>
          </Card>
        </>
      ) : (
        <>
          <div>
            <div style={{ fontSize: 17, fontWeight: 700 }}>
              {t(TAB_LABEL[tab])}
              <span style={{ marginLeft: 8, fontSize: 12, color: "var(--text-3)", fontWeight: 400 }}>{metaN == null ? "—" : String(metaN)}</span>
            </div>
            <div style={{ fontSize: 12, color: "var(--text-3)", maxWidth: 640 }}>{t("mkt.campaign.desc", { kind: t(TAB_LABEL[tab]) })}</div>
            <div style={{ fontSize: 12, color: "var(--text-3)", maxWidth: 640 }}>{t("mkt.status.note")}</div>
          </div>
          {formVisible ? (
            <div style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "flex-end" }}>
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
              <Button variant="primary" icon="plus" onClick={() => void createCampaign()}>
                {t("mkt.createCampaign")}
              </Button>
              <Button variant="quiet" onClick={() => setCreateOpen(false)}>
                {t("common.cancel")}
              </Button>
            </div>
          ) : null}
          <Card>
            <TableScroll>
            <Head>
              <span style={{ flex: 2 }}>{t("mkt.col.name")}</span>
              <span style={{ flex: 0.8 }}>{t("mkt.col.kind")}</span>
              <span style={{ flex: 0.9 }}>{t("mkt.col.status")}</span>
              <span style={{ flex: 1 }}>{t("mkt.col.channel")}</span>
              <span style={{ width: 110 }} />
            </Head>
            {state.showItems && !kindEmpty
              ? filtered.map((c) => (
                  <Head key={c.id} body>
                    <span style={{ flex: 2, fontWeight: 600 }}>{c.name}</span>
                    <span style={{ flex: 0.8, color: "var(--text-2)" }}>{t(TAB_LABEL[c.kind])}</span>
                    <span style={{ flex: 0.9 }}>{statusLabel(c.status, t)}</span>
                    <span style={{ flex: 1, color: "var(--text-3)" }}>{c.channel || "—"}</span>
                    <span style={{ width: 110, textAlign: "right" }}>
                      {c.status !== "done" && !blocked ? (
                        <Button
                          variant="quiet"
                          style={{ padding: "4px 8px" }}
                          title={t("mkt.patchDone.hint")}
                          onClick={() => void run(() => marketingApi.patchCampaign(c.id, { status: "done" }, org))}
                        >
                          {t("mkt.patchDone")}
                        </Button>
                      ) : null}
                    </span>
                  </Head>
                ))
              : null}
            {state.showEmpty ? <EmptyState>{t("mkt.emptyCampaign")}</EmptyState> : null}
            {kindEmpty ? <EmptyState>{t("mkt.emptyKind")}</EmptyState> : null}
            </TableScroll>
          </Card>
        </>
      )}
    </PageChrome>
  );
}

function sourceKey(source: AudienceSource): MsgKey {
  if (source === "file") return "mkt.source.file";
  if (source === "leadads") return "mkt.source.leadads";
  return "mkt.source.paste";
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
