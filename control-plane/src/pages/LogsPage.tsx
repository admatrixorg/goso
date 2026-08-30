import { useEffect, useMemo, useRef, useState } from "react";
import { backoffDelay, classifyStreamConn, clearLocalRows, historyStreamProvenance, streamStartBlocked, type StreamConn } from "../api/events-ops";
import { logsApi, type GatewayLog } from "../api/logs";
import {
  applyFilters,
  classifyLogsHistory,
  LOG_COMPONENTS,
  LOG_LEVELS,
  logsFilteredEmpty,
  mergeLive,
  toggleLevel,
  uniqueComponents,
  type LogLevel,
} from "../api/logs-ops";
import { formatStaleAt, listMetaCount } from "../api/page-state";
import { useI18n, type MsgKey } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { PageChrome } from "../ui/PageChrome";
import { PageStatus } from "../ui/PageStatus";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

const LEVEL_KEYS: Record<LogLevel, MsgKey> = {
  debug: "logs.level.debug",
  info: "logs.level.info",
  warn: "logs.level.warn",
  error: "logs.level.error",
};

function levelTone(level: string): "positive" | "warning" | "critical" | "neutral" | "accent" {
  switch (level) {
    case "error":
      return "critical";
    case "warn":
      return "warning";
    case "debug":
      return "neutral";
    default:
      return "accent";
  }
}

export function LogsPage() {
  const { t, locale } = useI18n();
  const [history, setHistory] = useState<GatewayLog[]>([]);
  const [liveRows, setLiveRows] = useState<GatewayLog[]>([]);
  const [extraComps, setExtraComps] = useState<string[]>([]);
  const [component, setComponent] = useState("");
  const [q, setQ] = useState("");
  const [levels, setLevels] = useState<LogLevel[]>([...LOG_LEVELS]);
  const [err, setErr] = useState<unknown>(null);
  const [streamErr, setStreamErr] = useState<unknown>(null);
  const [loading, setLoading] = useState(true);
  const [loaded, setLoaded] = useState(false);
  const [loadedAt, setLoadedAt] = useState<string | null>(null);
  const [live, setLive] = useState(false);
  const [paused, setPaused] = useState(false);
  const [conn, setConn] = useState<StreamConn>("off");
  const [retryIn, setRetryIn] = useState(0);
  const lastSeq = useRef(0);

  const filters = useMemo(() => ({ component: component || undefined, q: q || undefined, levels }), [component, q, levels]);
  const historyState = classifyLogsHistory({ loading, loaded, error: err, itemCount: history.length });
  const blocked = streamStartBlocked(historyState.kind);
  const streamConn = classifyStreamConn({ live, paused, conn });
  const shownHistory = useMemo(() => applyFilters(historyState.showItems ? history : [], filters), [history, filters, historyState.showItems]);
  const shownLive = useMemo(() => applyFilters(liveRows, filters), [liveRows, filters]);
  const components = useMemo(
    () => uniqueComponents([...(historyState.showItems ? history : []), ...liveRows], [...LOG_COMPONENTS, ...extraComps]),
    [history, liveRows, extraComps, historyState.showItems],
  );
  const filtersOn = Boolean(component || q.trim() || levels.length < LOG_LEVELS.length);
  const historyFilterEmpty = logsFilteredEmpty(historyState, history.length, shownHistory.length, filtersOn);
  const historyTrueEmpty = historyState.kind === "empty" && !filtersOn;
  const liveFilterEmpty = liveRows.length > 0 && shownLive.length === 0;
  const provenance = historyStreamProvenance(historyState.kind, streamConn);
  const metaN = listMetaCount(historyState.kind, shownHistory.length);

  async function load() {
    setLoading(true);
    try {
      const j = await logsApi.list({ limit: 100 });
      setHistory(j.logs);
      setExtraComps(j.components);
      let max = 0;
      for (const e of j.logs) {
        if (typeof e.seq === "number" && e.seq > max) max = e.seq;
      }
      if (max > lastSeq.current) lastSeq.current = max;
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
    void load();
  }, []);

  useEffect(() => {
    if (blocked) {
      setLive(false);
      setPaused(false);
      setConn("off");
      setRetryIn(0);
      return;
    }
    if (!live) {
      setConn("off");
      setRetryIn(0);
      return;
    }
    if (paused) {
      setConn("paused");
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
          await logsApi.stream(
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
      ? t("logs.connecting")
      : streamConn === "live"
        ? t("logs.connected")
        : streamConn === "paused"
          ? t("logs.paused")
          : streamConn === "reconnect"
            ? t("logs.reconnectIn", { s: retryIn || 1 })
            : streamConn === "error"
              ? t("logs.streamError")
              : t("logs.stop");

  const primary = paused ? (
    <Button
      variant="primary"
      disabled={blocked || !live}
      onClick={() => {
        if (!blocked) setPaused(false);
      }}
    >
      {t("logs.resume")}
    </Button>
  ) : live ? (
    <Button icon="refresh" iconGesture variant="primary" onClick={() => void load()} disabled={loading}>
      {t("common.refresh")}
    </Button>
  ) : (
    <Button
      variant="primary"
      disabled={blocked}
      onClick={() => {
        if (!blocked) setLive(true);
      }}
    >
      {t("logs.start")}
    </Button>
  );

  return (
    <PageChrome
      icon="list"
      title={t("logs.title")}
      description={t("logs.desc")}
      primary={primary}
      refresh={
        <>
          {!live || paused ? (
            <Button icon="refresh" iconGesture onClick={() => void load()} disabled={loading}>
              {t("common.refresh")}
            </Button>
          ) : null}
          {live ? (
            <Button
              variant="accent"
              disabled={blocked}
              onClick={() => {
                setLive(false);
                setPaused(false);
              }}
            >
              {t("logs.stop")}
            </Button>
          ) : null}
          <Button
            disabled={blocked || !live}
            onClick={() => {
              if (!blocked && live) setPaused((v) => !v);
            }}
          >
            {paused ? t("logs.resume") : t("logs.pause")}
          </Button>
          <Button
            variant="quiet"
            disabled={blocked || !live}
            title={t("logs.localHint")}
            onClick={() => {
              if (blocked || !live) return;
              setLiveRows(clearLocalRows(liveRows));
            }}
          >
            {t("logs.clearLocal")}
          </Button>
        </>
      }
      filters={
        <>
          <select className="z-field" value={component} disabled={blocked} onChange={(e) => setComponent(e.target.value)} aria-label={t("logs.filter.component")}>
            <option value="">{t("logs.component.all")}</option>
            {components.map((c) => (
              <option key={c} value={c}>
                {c}
              </option>
            ))}
          </select>
          <input
            className="z-field"
            placeholder={t("logs.filter.text")}
            value={q}
            disabled={blocked}
            onChange={(e) => setQ(e.target.value)}
            aria-label={t("logs.filter.text")}
            autoComplete="off"
          />
          <div role="group" aria-label={t("logs.filter.level")} style={{ display: "flex", gap: 6, flexWrap: "wrap" }}>
            {LOG_LEVELS.map((lv) => {
              const on = levels.includes(lv);
              return (
                <Button
                  key={lv}
                  variant={on ? "accent" : "quiet"}
                  disabled={blocked}
                  aria-pressed={on}
                  onClick={() => {
                    if (blocked) return;
                    setLevels((prev) => {
                      const next = toggleLevel(prev, lv);
                      return next.length === 0 ? prev : next;
                    });
                  }}
                >
                  {t(LEVEL_KEYS[lv])}
                </Button>
              );
            })}
          </div>
        </>
      }
    >
      <p role="note" style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>
        {t("logs.localHint")}
      </p>
      <PageStatus
        kind={historyState.kind}
        errorText={err ? `${t("logs.historyError")} · ${formatPublicError(err)}` : ""}
        staleAt={formatStaleAt(loadedAt, locale)}
        onReload={() => void load()}
      />
      {live && !blocked ? (
        <div data-page-state="stream" data-stream-conn={streamConn} role="status">
          {streamConn === "connecting" || streamConn === "reconnect" ? (
            <StatusLine kind="loading">{connLabel}</StatusLine>
          ) : streamConn === "error" || provenance === "stream" || provenance === "both" ? (
            <StatusLine kind="error">
              {t("logs.streamError")}
              {streamErr ? ` · ${formatPublicError(streamErr)}` : ""}
            </StatusLine>
          ) : (
            <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{connLabel}</p>
          )}
        </div>
      ) : null}
      {live && !blocked ? (
        <Card>
          <CardHeader icon="bolt" title={t("logs.list")} meta={`${connLabel} · ${t("logs.meta", { n: shownLive.length })}`} />
          <TableScroll>
            <div style={{ display: "flex", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)" }}>
              <span style={{ flex: 1.4 }}>{t("logs.col.ts")}</span>
              <span style={{ flex: 0.8 }}>{t("logs.col.level")}</span>
              <span style={{ flex: 1 }}>{t("logs.col.component")}</span>
              <span style={{ flex: 3.2 }}>{t("logs.col.message")}</span>
            </div>
            {streamConn === "connecting" && shownLive.length === 0 ? (
              <StatusLine kind="loading">{t("logs.connecting")}</StatusLine>
            ) : shownLive.length === 0 ? (
              <EmptyState data-page-state={liveFilterEmpty ? "filtered_empty" : "empty"}>
                {liveFilterEmpty ? t("logs.filterEmpty") : streamConn === "live" ? t("logs.waiting") : t("logs.empty")}
              </EmptyState>
            ) : (
              shownLive.map((e, i) => (
                <LogRow key={typeof e.seq === "number" ? `live:${e.seq}` : `live:${e.ts}:${i}`} e={e} />
              ))
            )}
          </TableScroll>
        </Card>
      ) : null}
      <Card>
        <CardHeader icon="history" title={t("logs.listHistory")} meta={metaN == null ? "—" : t("logs.meta", { n: metaN })} />
        <TableScroll>
          <div style={{ display: "flex", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)" }}>
            <span style={{ flex: 1.4 }}>{t("logs.col.ts")}</span>
            <span style={{ flex: 0.8 }}>{t("logs.col.level")}</span>
            <span style={{ flex: 1 }}>{t("logs.col.component")}</span>
            <span style={{ flex: 3.2 }}>{t("logs.col.message")}</span>
          </div>
          {historyState.showItems
            ? shownHistory.map((e, i) => (
                <LogRow key={typeof e.seq === "number" ? `hist:${e.seq}` : `hist:${e.ts}:${i}`} e={e} />
              ))
            : null}
          {historyTrueEmpty ? <EmptyState data-page-state="empty">{t("logs.historyEmpty")}</EmptyState> : null}
          {historyFilterEmpty ? <EmptyState data-page-state="filtered_empty">{t("logs.filterEmpty")}</EmptyState> : null}
        </TableScroll>
      </Card>
    </PageChrome>
  );
}

function LogRow({ e }: { e: GatewayLog }) {
  return (
    <div style={{ display: "flex", alignItems: "center", padding: "11px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 12.5 }}>
      <span style={{ flex: 1.4, color: "var(--text-3)", fontVariantNumeric: "tabular-nums" }}>{e.ts}</span>
      <span style={{ flex: 0.8 }}>
        <Badge tone={levelTone(e.level)}>{e.level.toUpperCase()}</Badge>
      </span>
      <span style={{ flex: 1, color: "var(--text-2)" }}>{e.component}</span>
      <span style={{ flex: 3.2, color: "var(--text-2)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", fontFamily: "var(--font-mono, inherit)" }}>
        {e.message}
      </span>
    </div>
  );
}
