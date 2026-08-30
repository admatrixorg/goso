import { useEffect, useMemo, useState } from "react";
import { activityApi, type ActivityRecord } from "../api/activity";
import {
  PAGE_SIZE,
  activityActionsBlocked,
  activityCursorMeta,
  activityFilteredEmpty,
  activityFiltersActive,
  classifyActivityList,
  parseDetail,
  publicHasSecrets,
  uniqueField,
  type ActivityPage,
} from "../api/activity-ops";
import { formatStaleAt, listMetaCount } from "../api/page-state";
import { useI18n } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { PageChrome } from "../ui/PageChrome";
import { PageStatus } from "../ui/PageStatus";
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
  const { t, locale } = useI18n();
  const [page, setPage] = useState<ActivityPage>({ records: [], total: 0, limit: PAGE_SIZE });
  const [action, setAction] = useState("");
  const [actor, setActor] = useState("");
  const [entity, setEntity] = useState("");
  const [ip, setIp] = useState("");
  const [range, setRange] = useState<TimeRange>("all");
  const [before, setBefore] = useState(0);
  const [stack, setStack] = useState<number[]>([]);
  const [err, setErr] = useState<unknown>(null);
  const [leak, setLeak] = useState(false);
  const [loading, setLoading] = useState(true);
  const [loaded, setLoaded] = useState(false);
  const [loadedAt, setLoadedAt] = useState<string | null>(null);
  const [open, setOpen] = useState("");
  const na = "—";

  const state = classifyActivityList({ loading, loaded, error: err, itemCount: page.records.length });
  const blocked = activityActionsBlocked(state.kind);
  const filtersOn = activityFiltersActive({ action, actor, entity, ip, range });
  const filterEmpty = activityFilteredEmpty(state, filtersOn);
  const trueEmpty = state.kind === "empty" && !filtersOn;
  const shown = state.showItems ? page.records : [];
  const actions = useMemo(() => uniqueField(shown, "action"), [shown]);
  const actors = useMemo(() => uniqueField(shown, "actor"), [shown]);
  const entities = useMemo(() => uniqueField(shown, "entity"), [shown]);
  const ips = useMemo(() => uniqueField(shown, "ip"), [shown]);
  const cursor = activityCursorMeta({ ...page, records: shown }, stack.length);
  const metaN = listMetaCount(state.kind, page.total);

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
      setLoaded(true);
      setLoadedAt(new Date().toISOString());
      setErr(null);
      setLeak((j.records || []).some((r) => publicHasSecrets(r)));
    } catch (e) {
      setErr(e);
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
    if (blocked) return;
    const next = page.next_before || 0;
    if (!next) return;
    setStack((s) => [...s, before]);
    setBefore(next);
    void load(next);
  }

  function goPrev() {
    if (blocked) return;
    const prev = stack[stack.length - 1] || 0;
    setStack((s) => s.slice(0, -1));
    setBefore(prev);
    void load(prev);
  }

  return (
    <PageChrome
      icon="shield"
      title={t("activity.title")}
      description={t("activity.desc")}
      primary={
        <Button icon="refresh" iconGesture variant="primary" onClick={() => void load(before)} disabled={loading}>
          {t("common.refresh")}
        </Button>
      }
      filters={
        <>
          <input
            className="z-field"
            list="activity-actions"
            placeholder={t("activity.filter.action")}
            value={action}
            disabled={blocked}
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
            disabled={blocked}
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
            disabled={blocked}
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
            disabled={blocked}
            onChange={(e) => setIp(e.target.value)}
            aria-label={t("activity.filter.ip")}
            autoComplete="off"
          />
          <datalist id="activity-ips">
            {ips.map((a) => (
              <option key={a} value={a} />
            ))}
          </datalist>
          <select className="z-field" value={range} disabled={blocked} onChange={(e) => setRange(e.target.value as TimeRange)} aria-label={t("activity.filter.range")}>
            <option value="all">{t("activity.range.all")}</option>
            <option value="1h">{t("activity.range.1h")}</option>
            <option value="24h">{t("activity.range.24h")}</option>
            <option value="7d">{t("activity.range.7d")}</option>
          </select>
        </>
      }
    >
      <p role="note" style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>
        {t("activity.noLive")} {t("activity.noExport")}
      </p>
      <PageStatus kind={state.kind} errorText={err ? formatPublicError(err) : ""} staleAt={formatStaleAt(loadedAt, locale)} onReload={() => void load(before)} />
      {leak ? <StatusLine kind="error">{t("activity.leak")}</StatusLine> : null}
      <Card>
        <CardHeader icon="history" title={t("activity.list")} meta={metaN == null ? "—" : t("activity.meta", { n: metaN })} />
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
          {shown.map((e) => (
            <ActivityRow key={e.id || String(e.seq)} row={e} open={open} onOpen={blocked ? () => undefined : setOpen} na={na} />
          ))}
          {trueEmpty ? <EmptyState data-page-state="empty">{t("activity.empty")}</EmptyState> : null}
          {filterEmpty ? <EmptyState data-page-state="filtered_empty">{t("activity.filterEmpty")}</EmptyState> : null}
        </TableScroll>
        <div style={{ display: "flex", gap: 8, alignItems: "center", padding: "10px 16px", flexWrap: "wrap" }}>
          <Button disabled={blocked || loading || !cursor.hasPrev} onClick={goPrev}>
            {t("activity.prev")}
          </Button>
          <span style={{ fontSize: 12.5, color: "var(--text-3)" }}>
            {metaN == null
              ? "—"
              : t("activity.page", { n: cursor.shown, total: page.total })}
          </span>
          <Button disabled={blocked || loading || !cursor.hasNext} onClick={goNext}>
            {t("activity.next")}
          </Button>
          {state.showItems && (cursor.before || cursor.nextBefore) ? (
            <span style={{ fontSize: 12, color: "var(--text-3)" }}>
              {t("activity.cursor", { before: cursor.before ?? 0, next: cursor.nextBefore ?? "—" })}
            </span>
          ) : null}
        </div>
      </Card>
    </PageChrome>
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
