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
import { Button } from "../ui/Button";
import { DataTable, THead, Th, Td, Tr } from "../ui/DataTable";
import { EmptyState } from "../ui/EmptyState";
import { PageChrome } from "../ui/PageChrome";
import { PageStatus } from "../ui/PageStatus";
import { StatusBadge, type Status } from "../ui/StatusBadge";
import { StatusLine, formatPublicError } from "../ui/StatusLine";
import { fieldClass, selectClass } from "../ui/field";

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

function actionStatus(action: string): Status {
  if (action.includes("delete") || action.includes("revoke") || action.includes("clear")) return "error";
  if (action.includes("create") || action.includes("approve")) return "success";
  if (action.includes("rotate") || action.includes("update") || action.includes("merge")) return "info";
  if (action.includes("deny") || action.includes("disconnect")) return "warning";
  return "default";
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
      icon="activity"
      title={t("activity.title")}
      description={t("activity.desc")}
      refresh={
        <Button icon="refresh" iconGesture variant="outline" onClick={() => void load(before)} disabled={loading}>
          {t("common.refresh")}
        </Button>
      }
      filters={
        <>
          <div className="max-w-sm min-w-40 flex-1">
            <input
              className={fieldClass}
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
          </div>
          <div className="max-w-sm min-w-40 flex-1">
            <input
              className={fieldClass}
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
          </div>
          <div className="max-w-sm min-w-40 flex-1">
            <input
              className={fieldClass}
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
          </div>
          <div className="max-w-sm min-w-40 flex-1">
            <input
              className={fieldClass}
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
          </div>
          <select className={selectClass} value={range} disabled={blocked} onChange={(e) => setRange(e.target.value as TimeRange)} aria-label={t("activity.filter.range")}>
            <option value="all">{t("activity.range.all")}</option>
            <option value="1h">{t("activity.range.1h")}</option>
            <option value="24h">{t("activity.range.24h")}</option>
            <option value="7d">{t("activity.range.7d")}</option>
          </select>
        </>
      }
    >
      <p role="note" className="text-sm text-muted-foreground">
        {t("activity.noLive")} {t("activity.noExport")}
      </p>
      <PageStatus kind={state.kind} errorText={err ? formatPublicError(err) : ""} staleAt={formatStaleAt(loadedAt, locale)} onReload={() => void load(before)} />
      {leak ? <StatusLine kind="error">{t("activity.leak")}</StatusLine> : null}
      {trueEmpty ? <EmptyState icon="activity" title={t("activity.empty")} data-page-state="empty" /> : null}
      {filterEmpty ? <EmptyState icon="search" title={t("activity.filterEmpty")} data-page-state="filtered_empty" /> : null}
      {state.showItems ? (
        <DataTable minWidth="800px">
          <THead>
            <Th>{t("activity.col.ts")}</Th>
            <Th>{t("activity.col.action")}</Th>
            <Th>{t("activity.col.actor")}</Th>
            <Th>{t("activity.col.entity")}</Th>
            <Th>{t("activity.col.entityId")}</Th>
            <Th>{t("activity.col.ip")}</Th>
          </THead>
          <tbody>
            {shown.map((e) => (
              <ActivityRow key={e.id || String(e.seq)} row={e} open={open} onOpen={blocked ? () => undefined : setOpen} na={na} />
            ))}
          </tbody>
        </DataTable>
      ) : null}
      <div className="flex flex-wrap items-center gap-2">
        <Button variant="outline" size="sm" disabled={blocked || loading || !cursor.hasPrev} onClick={goPrev}>
          {t("activity.prev")}
        </Button>
        <span className="text-sm text-muted-foreground">
          {metaN == null ? "—" : t("activity.page", { n: cursor.shown, total: page.total })}
        </span>
        <Button variant="outline" size="sm" disabled={blocked || loading || !cursor.hasNext} onClick={goNext}>
          {t("activity.next")}
        </Button>
        {state.showItems && (cursor.before || cursor.nextBefore) ? (
          <span className="text-xs text-muted-foreground">{t("activity.cursor", { before: cursor.before ?? 0, next: cursor.nextBefore ?? "—" })}</span>
        ) : null}
      </div>
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
    <>
      <Tr selected={open === key} onClick={() => onOpen(open === key ? "" : key)} aria-expanded={open === key}>
        <Td className="tabular-nums text-muted-foreground">{row.ts || na}</Td>
        <Td>
          <StatusBadge status={actionStatus(row.action)} label={row.action || na} />
        </Td>
        <Td className="text-muted-foreground">{row.actor || na}</Td>
        <Td className="font-semibold">{row.entity || na}</Td>
        <Td className="max-w-[180px] truncate text-muted-foreground">{row.entity_id || na}</Td>
        <Td className="tabular-nums text-muted-foreground">{row.ip || na}</Td>
      </Tr>
      {open === key ? (
        <tr>
          <td colSpan={6} className="px-4 py-3" aria-label={t("activity.detail")}>
            {details.length === 0 ? (
              <p className="m-0 text-sm text-muted-foreground">{t("activity.noDetail")}</p>
            ) : (
              <dl className="m-0 grid grid-cols-[140px_1fr] gap-x-3 gap-y-1 text-sm">
                {details.map((d) => (
                  <span key={d.key} className="contents">
                    <dt className="m-0 text-muted-foreground">{d.key}</dt>
                    <dd className="m-0 break-words">{d.value}</dd>
                  </span>
                ))}
              </dl>
            )}
          </td>
        </tr>
      ) : null}
    </>
  );
}
