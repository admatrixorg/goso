import { useEffect, useMemo, useState } from "react";
import { activityApi, type ActivityRecord } from "../api/activity";
import {
  PAGE_SIZE,
  parseDetail,
  publicHasSecrets,
  uniqueField,
  type ActivityPage,
} from "../api/activity-ops";
import { useI18n } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { SectionHeader } from "../ui/SectionHeader";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

type TimeRange = "1h" | "24h" | "7d" | "all";

function rangeSince(range: TimeRange, now = Date.now()): string | undefined {
  const ms: Record<Exclude<TimeRange, "all">, number> = {
    "1h": 3_600_000,
    "24h": 86_400_000,
    "7d": 7 * 86_400_000,
  };
  if (range === "all") return undefined;
  return new Date(now - ms[range]).toISOString();
}

function actionTone(action: string): "positive" | "warning" | "critical" | "neutral" | "accent" {
  if (action.includes("delete") || action.includes("revoke") || action.includes("clear")) return "critical";
  if (action.includes("create") || action.includes("approve")) return "positive";
  if (action.includes("rotate") || action.includes("update") || action.includes("merge")) return "accent";
  if (action.includes("deny") || action.includes("disconnect")) return "warning";
  return "neutral";
}

export function ActivityPage() {
  const { t } = useI18n();
  const [page, setPage] = useState<ActivityPage>({ records: [], total: 0, limit: PAGE_SIZE });
  const [action, setAction] = useState("");
  const [actor, setActor] = useState("");
  const [entity, setEntity] = useState("");
  const [ip, setIp] = useState("");
  const [range, setRange] = useState<TimeRange>("all");
  const [before, setBefore] = useState(0);
  const [stack, setStack] = useState<number[]>([]);
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(true);
  const [open, setOpen] = useState("");
  const na = "—";

  const actions = useMemo(() => uniqueField(page.records, "action"), [page.records]);
  const actors = useMemo(() => uniqueField(page.records, "actor"), [page.records]);
  const entities = useMemo(() => uniqueField(page.records, "entity"), [page.records]);
  const ips = useMemo(() => uniqueField(page.records, "ip"), [page.records]);
  const filtered = Boolean(action || actor || entity || ip || range !== "all");

  async function load(nextBefore = before) {
    setLoading(true);
    try {
      const j = await activityApi.list({
        action: action || undefined,
        actor: actor || undefined,
        entity: entity || undefined,
        ip: ip || undefined,
        since: rangeSince(range),
        limit: PAGE_SIZE,
        before: nextBefore || undefined,
      });
      setPage(j);
      const leak = (j.records || []).some((r) => publicHasSecrets(r));
      setErr(leak ? t("activity.leak") : "");
    } catch (e) {
      setErr(formatPublicError(e));
      setPage({ records: [], total: 0, limit: PAGE_SIZE });
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    setBefore(0);
    setStack([]);
    setOpen("");
    void load(0);
  }, [action, actor, entity, ip, range]);

  function goNext() {
    const cursor = page.next_before || 0;
    if (!cursor) return;
    setStack((s) => [...s, before]);
    setBefore(cursor);
    void load(cursor);
  }

  function goPrev() {
    const prev = stack[stack.length - 1] || 0;
    setStack((s) => s.slice(0, -1));
    setBefore(prev);
    void load(prev);
  }

  return (
    <div style={{ padding: "14px 22px 40px", display: "flex", flexDirection: "column", gap: 14 }}>
      <SectionHeader
        icon="shield"
        title={t("activity.title")}
        description={t("activity.desc")}
        actions={
          <Button icon="refresh" iconGesture onClick={() => void load(before)}>
            {t("common.refresh")}
          </Button>
        }
      />
      {err ? <StatusLine kind="error">{err}</StatusLine> : null}
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
        <input
          className="z-field"
          list="activity-actions"
          placeholder={t("activity.filter.action")}
          value={action}
          onChange={(e) => setAction(e.target.value)}
          aria-label={t("activity.filter.action")}
          autoComplete="off"
        />
        <datalist id="activity-actions">
          {actions.map((a) => (
            <option key={a} value={a} />
          ))}
        </datalist>
        <input
          className="z-field"
          list="activity-actors"
          placeholder={t("activity.filter.actor")}
          value={actor}
          onChange={(e) => setActor(e.target.value)}
          aria-label={t("activity.filter.actor")}
          autoComplete="off"
        />
        <datalist id="activity-actors">
          {actors.map((a) => (
            <option key={a} value={a} />
          ))}
        </datalist>
        <input
          className="z-field"
          list="activity-entities"
          placeholder={t("activity.filter.entity")}
          value={entity}
          onChange={(e) => setEntity(e.target.value)}
          aria-label={t("activity.filter.entity")}
          autoComplete="off"
        />
        <datalist id="activity-entities">
          {entities.map((a) => (
            <option key={a} value={a} />
          ))}
        </datalist>
        <input
          className="z-field"
          list="activity-ips"
          placeholder={t("activity.filter.ip")}
          value={ip}
          onChange={(e) => setIp(e.target.value)}
          aria-label={t("activity.filter.ip")}
          autoComplete="off"
        />
        <datalist id="activity-ips">
          {ips.map((a) => (
            <option key={a} value={a} />
          ))}
        </datalist>
        <select className="z-field" value={range} onChange={(e) => setRange(e.target.value as TimeRange)} aria-label={t("activity.filter.range")}>
          <option value="all">{t("activity.range.all")}</option>
          <option value="1h">{t("activity.range.1h")}</option>
          <option value="24h">{t("activity.range.24h")}</option>
          <option value="7d">{t("activity.range.7d")}</option>
        </select>
      </div>
      <Card>
        <CardHeader icon="history" title={t("activity.list")} meta={t("activity.meta", { n: page.total })} />
        <TableScroll>
          <div
            style={{
              display: "flex",
              padding: "8px 16px",
              borderBottom: "1px solid var(--border-soft)",
              fontSize: 10,
              fontWeight: 600,
              letterSpacing: ".4px",
              color: "var(--text-3)",
            }}
          >
            <span style={{ flex: 1.4 }}>{t("activity.col.ts")}</span>
            <span style={{ flex: 0.9 }}>{t("activity.col.action")}</span>
            <span style={{ flex: 1 }}>{t("activity.col.actor")}</span>
            <span style={{ flex: 0.9 }}>{t("activity.col.entity")}</span>
            <span style={{ flex: 1.1 }}>{t("activity.col.entityId")}</span>
            <span style={{ flex: 1 }}>{t("activity.col.ip")}</span>
          </div>
          {page.records.map((e) => (
            <ActivityRow key={e.id || String(e.seq)} row={e} open={open} onOpen={setOpen} na={na} />
          ))}
          {loading ? <StatusLine kind="loading" /> : null}
          {!loading && page.records.length === 0 ? (
            <EmptyState>{filtered ? t("activity.filterEmpty") : t("activity.empty")}</EmptyState>
          ) : null}
        </TableScroll>
        <div style={{ display: "flex", gap: 8, alignItems: "center", padding: "10px 16px", flexWrap: "wrap" }}>
          <Button disabled={loading || stack.length === 0} onClick={goPrev}>
            {t("activity.prev")}
          </Button>
          <span style={{ fontSize: 12.5, color: "var(--text-3)" }}>
            {t("activity.page", { n: page.records.length, total: page.total })}
          </span>
          <Button disabled={loading || !page.next_before} onClick={goNext}>
            {t("activity.next")}
          </Button>
        </div>
      </Card>
    </div>
  );
}

function ActivityRow({
  row,
  open,
  onOpen,
  na,
}: {
  row: ActivityRecord;
  open: string;
  onOpen: (key: string) => void;
  na: string;
}) {
  const { t } = useI18n();
  const key = row.id || String(row.seq);
  const details = open === key ? parseDetail(row) : [];
  return (
    <div style={{ borderBottom: "1px solid var(--border-soft)" }}>
      <button
        type="button"
        onClick={() => onOpen(open === key ? "" : key)}
        aria-expanded={open === key}
        style={{
          display: "flex",
          alignItems: "center",
          width: "100%",
          padding: "11px 16px",
          fontSize: 12.5,
          background: "transparent",
          border: 0,
          color: "inherit",
          textAlign: "left",
          cursor: "pointer",
          fontFamily: "inherit",
        }}
      >
        <span style={{ flex: 1.4, color: "var(--text-3)", fontVariantNumeric: "tabular-nums" }}>{row.ts || na}</span>
        <span style={{ flex: 0.9 }}>
          <Badge tone={actionTone(row.action)}>{row.action || na}</Badge>
        </span>
        <span style={{ flex: 1, color: "var(--text-2)" }}>{row.actor || na}</span>
        <span style={{ flex: 0.9 }}>{row.entity || na}</span>
        <span style={{ flex: 1.1, color: "var(--text-2)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
          {row.entity_id || na}
        </span>
        <span style={{ flex: 1, color: "var(--text-3)", fontVariantNumeric: "tabular-nums" }}>{row.ip || na}</span>
      </button>
      {open === key ? (
        <div style={{ padding: "0 16px 12px" }} aria-label={t("activity.detail")}>
          {details.length === 0 ? (
            <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("activity.noDetail")}</p>
          ) : (
            <dl style={{ margin: 0, display: "grid", gridTemplateColumns: "140px 1fr", gap: "4px 12px", fontSize: 12.5 }}>
              {details.map((d) => (
                <span key={d.key} style={{ display: "contents" }}>
                  <dt style={{ color: "var(--text-3)", margin: 0 }}>{d.key}</dt>
                  <dd style={{ margin: 0, color: "var(--text-2)", wordBreak: "break-word" }}>{d.value}</dd>
                </span>
              ))}
            </dl>
          )}
        </div>
      ) : null}
    </div>
  );
}
