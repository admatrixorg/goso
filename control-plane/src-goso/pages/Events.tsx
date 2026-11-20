import { Fragment, useEffect, useMemo, useRef, useState } from "react";
import { eventsApi, type GatewayEvent } from "../api/events";
import {
  applyFilters,
  backoffDelay,
  classifyEventsHistory,
  classifyStreamConn,
  clearLocalRows,
  EVENT_TYPES,
  eventKey,
  eventsFilteredEmpty,
  eventsLiveFilteredEmpty,
  historyStreamProvenance,
  mergeLive,
  parseDetail,
  streamStartBlocked,
  uniqueActors,
  type EventType,
  type StreamConn,
} from "../api/events-ops";
import { formatStaleAt, listMetaCount } from "../api/page-state";
import { useI18n, type MsgKey } from "../i18n";
import { Button } from "../ui/Button";
import { Card, CardHeader } from "../ui/Card";
import { DataTable, THead, Th, Td, Tr } from "../ui/DataTable";
import { EmptyState } from "../ui/EmptyState";
import { PageChrome } from "../ui/PageChrome";
import { PageStatus } from "../ui/PageStatus";
import { StatusBadge, type Status } from "../ui/StatusBadge";
import { StatusLine, formatPublicError } from "../ui/StatusLine";
import { fieldClass, selectClass } from "../ui/field";

function kindStatus(k: string): Status {
  if (k.includes("error") || k.includes("fail") || k.includes("delete")) return "error";
  if (k.includes("success") || k.includes("ok") || k.includes("create")) return "success";
  if (k.includes("pending") || k.includes("approval")) return "warning";
  if (k.includes("attempt") || k.includes("update")) return "info";
  return "default";
}

const TYPE_KEYS: Record<EventType, MsgKey> = {
  connector: "events.type.connector",
  agent: "events.type.agent",
  team: "events.type.team",
  task: "events.type.task",
  message: "events.type.message",
  agent_link: "events.type.agent_link",
};

function EventRows({
  rows,
  open,
  onOpen,
  empty,
  emptyState,
}: {
  rows: GatewayEvent[];
  open: string;
  onOpen: (key: string) => void;
  empty?: string;
  emptyState?: string;
}) {
  const { t } = useI18n();
  if (rows.length === 0) {
    return empty ? (
      <EmptyState
        icon={emptyState === "filtered_empty" ? "search" : "rss"}
        title={empty}
        data-page-state={emptyState || "empty"}
      />
    ) : null;
  }
  return (
    <DataTable minWidth="960px" className="mt-0 rounded-none border-0">
      <THead>
        <Th>{t("events.col.ts")}</Th>
        <Th>{t("events.col.type")}</Th>
        <Th>{t("events.col.kind")}</Th>
        <Th>{t("events.col.actor")}</Th>
        <Th>{t("events.col.connector")}</Th>
        <Th>{t("events.col.tool")}</Th>
        <Th>{t("events.col.summary")}</Th>
      </THead>
      <tbody>
        {rows.map((e, i) => {
          const key = eventKey(e, i);
          const details = open === key ? parseDetail(e) : [];
          return (
            <Fragment key={key}>
              <Tr
                selected={open === key}
                onClick={() => onOpen(open === key ? "" : key)}
                aria-expanded={open === key}
              >
                <Td className="tabular-nums text-muted-foreground">{e.ts}</Td>
                <Td>
                  <StatusBadge status="default" label={e.type || "connector"} />
                </Td>
                <Td>
                  <StatusBadge status={kindStatus(e.kind)} label={e.kind} />
                </Td>
                <Td className="text-muted-foreground">{e.actor || e.agent_id || e.team_id || "—"}</Td>
                <Td>{e.connector || "—"}</Td>
                <Td className="text-muted-foreground">{e.tool || e.action || "—"}</Td>
                <Td className="max-w-xs truncate text-muted-foreground">{e.summary}</Td>
              </Tr>
              {open === key ? (
                <Tr>
                  <Td colSpan={7} onClick={(ev) => ev.stopPropagation()}>
                    <div className="py-1" aria-label={t("events.detail")}>
                      {details.length === 0 ? (
                        <p className="text-sm text-muted-foreground">{e.summary || t("events.noDetail")}</p>
                      ) : (
                        <dl className="grid grid-cols-[140px_1fr] gap-x-3 gap-y-1 text-sm">
                          {details.map((d) => (
                            <Fragment key={d.key}>
                              <dt className="m-0 text-muted-foreground">{d.key}</dt>
                              <dd className="m-0 break-words">{d.value}</dd>
                            </Fragment>
                          ))}
                        </dl>
                      )}
                    </div>
                  </Td>
                </Tr>
              ) : null}
            </Fragment>
          );
        })}
      </tbody>
    </DataTable>
  );
}

export function EventsPage() {
  const { t, locale } = useI18n();
  const [history, setHistory] = useState<GatewayEvent[]>([]);
  const [liveRows, setLiveRows] = useState<GatewayEvent[]>([]);
  const [kind, setKind] = useState("");
  const [connector, setConnector] = useState("");
  const [type, setType] = useState("");
  const [actor, setActor] = useState("");
  const [err, setErr] = useState<unknown>(null);
  const [streamErr, setStreamErr] = useState<unknown>(null);
  const [loading, setLoading] = useState(true);
  const [loaded, setLoaded] = useState(false);
  const [loadedAt, setLoadedAt] = useState<string | null>(null);
  const [live, setLive] = useState(false);
  const [paused, setPaused] = useState(false);
  const [conn, setConn] = useState<StreamConn>("off");
  const [retryIn, setRetryIn] = useState(0);
  const [open, setOpen] = useState("");
  const lastSeq = useRef(0);

  const filters = useMemo(() => ({ type: type || undefined, actor: actor || undefined, kind: kind || undefined, connector: connector || undefined }), [type, actor, kind, connector]);
  const historyState = classifyEventsHistory({ loading, loaded, error: err, itemCount: history.length });
  const blocked = streamStartBlocked(historyState.kind);
  const streamConn = classifyStreamConn({ live, paused, conn });
  const shownHistory = useMemo(() => applyFilters(historyState.showItems ? history : [], filters), [history, filters, historyState.showItems]);
  const shownLive = useMemo(() => applyFilters(liveRows, filters), [liveRows, filters]);
  const actors = useMemo(() => uniqueActors([...liveRows, ...(historyState.showItems ? history : [])]), [liveRows, history, historyState.showItems]);
  const filtersOn = Boolean(type || actor || kind || connector);
  const historyFilterEmpty = eventsFilteredEmpty(historyState, filtersOn);
  const historyTrueEmpty = historyState.kind === "empty" && !filtersOn;
  const liveFilterEmpty = eventsLiveFilteredEmpty(liveRows.length, shownLive.length);
  const provenance = historyStreamProvenance(historyState.kind, streamConn);
  const metaN = listMetaCount(historyState.kind, shownHistory.length);

  async function load() {
    setLoading(true);
    try {
      const j = await eventsApi.list({
        kind: kind || undefined,
        connector: connector || undefined,
        type: type || undefined,
        actor: actor || undefined,
        limit: 100,
      });
      setHistory(j.events);
      setLoaded(true);
      setLoadedAt(new Date().toISOString());
      setErr(null);
    } catch (e) {
      setErr(e);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    const timer = setTimeout(() => void load(), history.length === 0 ? 0 : 250);
    return () => clearTimeout(timer);
  }, [type, actor, kind, connector]);

  useEffect(() => {
    if (blocked) {
      setLive(false);
      setPaused(false);
      setConn("off");
      setRetryIn(0);
      return;
    }
    if (!live || paused) {
      setConn(live ? "paused" : "off");
      setRetryIn(0);
      return;
    }
    let stopped = false;
    let attempt = 0;
    let ctrl: AbortController | undefined;
    let waitTimer: ReturnType<typeof setTimeout> | undefined;
    let waitDone: (() => void) | undefined;
    const wait = (ms: number) =>
      new Promise<void>((resolve) => {
        waitDone = resolve;
        setRetryIn(Math.ceil(ms / 1000));
        waitTimer = setTimeout(resolve, ms);
      });
    const run = async () => {
      while (!stopped) {
        ctrl = new AbortController();
        setConn("connecting");
        setRetryIn(0);
        try {
          await eventsApi.stream(
            undefined,
            (e) => {
              if (typeof e.seq === "number" && e.seq > lastSeq.current) lastSeq.current = e.seq;
              setLiveRows((prev) => mergeLive(prev, e));
            },
            () => {
              attempt = 0;
              setConn("live");
              setStreamErr(null);
            },
            ctrl.signal,
            lastSeq.current || undefined,
          );
          if (stopped) return;
          attempt += 1;
          const delay = backoffDelay(attempt - 1);
          setConn("reconnect");
          await wait(delay);
        } catch (e) {
          if (stopped || ctrl.signal.aborted) return;
          attempt += 1;
          setConn("error");
          setStreamErr(e);
          const delay = backoffDelay(attempt - 1);
          setConn("reconnect");
          await wait(delay);
        }
      }
    };
    void run();
    return () => {
      stopped = true;
      ctrl?.abort();
      if (waitTimer) clearTimeout(waitTimer);
      waitDone?.();
    };
  }, [live, paused, blocked]);

  const connLabel =
    streamConn === "connecting"
      ? t("events.connecting")
      : streamConn === "live"
        ? t("events.connected")
        : streamConn === "paused"
          ? t("events.paused")
          : streamConn === "reconnect"
            ? t("events.reconnectIn", { s: retryIn || 1 })
            : streamConn === "error"
              ? t("events.streamError")
              : t("events.liveOff");

  const primary = paused ? (
    <Button
      variant="default"
      disabled={blocked || !live}
      onClick={() => {
        if (!blocked) setPaused(false);
      }}
    >
      {t("events.resume")}
    </Button>
  ) : undefined;

  return (
    <PageChrome
      icon="rss"
      title={t("events.title")}
      description={t("events.desc")}
      primary={primary}
      refresh={
        <>
          <Button icon="refresh" iconGesture variant="outline" onClick={() => void load()} disabled={loading}>
            {t("common.refresh")}
          </Button>
          <Button
            variant={live ? "outline" : "secondary"}
            disabled={blocked}
            onClick={() => {
              if (blocked) return;
              setLive((v) => !v);
              if (live) setPaused(false);
            }}
          >
            {live ? t("events.liveOn") : t("events.liveOff")}
          </Button>
          <Button
            variant="outline"
            disabled={blocked || !live}
            onClick={() => {
              if (!blocked && live) setPaused((v) => !v);
            }}
          >
            {paused ? t("events.resume") : t("events.pause")}
          </Button>
          <Button
            variant="ghost"
            className="text-destructive"
            disabled={blocked || !live}
            title={t("events.localHint")}
            onClick={() => {
              if (blocked || !live) return;
              setLiveRows(clearLocalRows(liveRows));
              setOpen("");
            }}
          >
            {t("events.clearLocal")}
          </Button>
        </>
      }
      filters={
        <>
          <select className={selectClass} value={type} disabled={blocked} onChange={(e) => setType(e.target.value)} aria-label={t("events.filterType")}>
            <option value="">{t("events.type.all")}</option>
            {EVENT_TYPES.map((tp) => (
              <option key={tp} value={tp}>
                {t(TYPE_KEYS[tp])}
              </option>
            ))}
          </select>
          <input
            className={fieldClass}
            list="event-actors"
            placeholder={t("events.filterActor")}
            value={actor}
            disabled={blocked}
            onChange={(e) => setActor(e.target.value)}
            aria-label={t("events.filterActor")}
            autoComplete="off"
          />
          <datalist id="event-actors">
            {actors.map((a) => (
              <option key={a} value={a} />
            ))}
          </datalist>
          <input
            className={fieldClass}
            placeholder={t("events.filterKind")}
            value={kind}
            disabled={blocked}
            onChange={(e) => setKind(e.target.value)}
            aria-label={t("events.filterKind")}
          />
          <input
            className={fieldClass}
            placeholder={t("events.filterConnector")}
            value={connector}
            disabled={blocked}
            onChange={(e) => setConnector(e.target.value)}
            aria-label={t("events.filterConnector")}
          />
        </>
      }
    >
      <p role="note" className="text-sm text-muted-foreground">
        {t("events.auditHint")} {t("events.localHint")}
      </p>
      <PageStatus
        kind={historyState.kind}
        errorText={err ? `${t("events.historyError")} · ${formatPublicError(err)}` : ""}
        staleAt={formatStaleAt(loadedAt, locale)}
        onReload={() => void load()}
      />
      {live && !blocked ? (
        <div data-page-state="stream" data-stream-conn={streamConn} role="status">
          {streamConn === "connecting" || streamConn === "reconnect" ? (
            <StatusLine kind="loading">{connLabel}</StatusLine>
          ) : streamConn === "error" || provenance === "stream" || provenance === "both" ? (
            <StatusLine kind="error">
              {t("events.streamError")}
              {streamErr ? ` · ${formatPublicError(streamErr)}` : ""}
            </StatusLine>
          ) : (
            <p className="text-sm text-muted-foreground">{connLabel}</p>
          )}
        </div>
      ) : null}
      {live && !blocked ? (
        <Card>
          <CardHeader icon="bolt" title={t("events.liveList")} meta={`${connLabel} · ${t("events.meta", { n: shownLive.length })}`} />
          {streamConn === "connecting" && shownLive.length === 0 ? (
            <StatusLine kind="loading">{t("events.connecting")}</StatusLine>
          ) : (
            <EventRows
              rows={shownLive}
              open={open}
              onOpen={setOpen}
              empty={
                liveFilterEmpty
                  ? t("events.filterEmpty")
                  : streamConn === "live"
                    ? t("events.waiting")
                    : streamConn === "paused"
                      ? t("events.paused")
                      : t("events.liveEmpty")
              }
              emptyState={liveFilterEmpty ? "filtered_empty" : "empty"}
            />
          )}
        </Card>
      ) : null}
      <Card>
        <CardHeader icon="history" title={t("events.list")} meta={metaN == null ? "—" : t("events.meta", { n: metaN })} />
        {historyState.showItems ? (
          <EventRows
            rows={shownHistory}
            open={open}
            onOpen={setOpen}
            empty={shownHistory.length === 0 && history.length > 0 ? t("events.filterEmpty") : undefined}
            emptyState="filtered_empty"
          />
        ) : null}
        {historyTrueEmpty ? <EmptyState icon="rss" title={t("events.empty")} data-page-state="empty" /> : null}
        {historyFilterEmpty ? <EmptyState icon="search" title={t("events.filterEmpty")} data-page-state="filtered_empty" /> : null}
      </Card>
    </PageChrome>
  );
}
