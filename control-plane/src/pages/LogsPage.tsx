import { useEffect, useMemo, useRef, useState } from "react";
import { logsApi, type GatewayLog } from "../api/logs";
import {
  applyFilters,
  LOG_COMPONENTS,
  LOG_LEVELS,
  mergeLive,
  toggleLevel,
  uniqueComponents,
  type LogLevel,
  type StreamConn,
} from "../api/logs-ops";
import { backoffDelay } from "../api/events-ops";
import { useI18n, type MsgKey } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { SectionHeader } from "../ui/SectionHeader";
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
  const { t } = useI18n();
  const [rows, setRows] = useState<GatewayLog[]>([]);
  const [extraComps, setExtraComps] = useState<string[]>([]);
  const [component, setComponent] = useState("");
  const [q, setQ] = useState("");
  const [levels, setLevels] = useState<LogLevel[]>([...LOG_LEVELS]);
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(false);
  const [live, setLive] = useState(false);
  const [paused, setPaused] = useState(false);
  const [conn, setConn] = useState<StreamConn>("off");
  const [retryIn, setRetryIn] = useState(0);
  const lastSeq = useRef(0);
  const seeded = useRef(false);
  const seedGen = useRef(0);

  const filters = useMemo(
    () => ({ component: component || undefined, q: q || undefined, levels }),
    [component, q, levels],
  );
  const shown = useMemo(() => applyFilters(rows, filters), [rows, filters]);
  const components = useMemo(
    () => uniqueComponents(rows, [...LOG_COMPONENTS, ...extraComps]),
    [rows, extraComps],
  );

  useEffect(() => {
    if (!live) {
      seeded.current = false;
      lastSeq.current = 0;
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
    const seed = async () => {
      if (seeded.current) return;
      const gen = seedGen.current;
      setLoading(true);
      try {
        const j = await logsApi.list({ limit: 100 });
        if (stopped || gen !== seedGen.current) return;
        setRows(j.logs);
        setExtraComps(j.components);
        let max = 0;
        for (const e of j.logs) {
          if (typeof e.seq === "number" && e.seq > max) max = e.seq;
        }
        lastSeq.current = max;
        setErr("");
        seeded.current = true;
      } catch (e) {
        if (!stopped) setErr(formatPublicError(e));
      } finally {
        if (!stopped) setLoading(false);
      }
    };
    const run = async () => {
      await seed();
      while (!stopped) {
        ctrl = new AbortController();
        setConn("connecting");
        setRetryIn(0);
        try {
          await logsApi.stream(
            (e) => {
              if (typeof e.seq === "number" && e.seq > lastSeq.current) lastSeq.current = e.seq;
              setRows((prev) => mergeLive(prev, e));
            },
            () => {
              attempt = 0;
              setConn("live");
              setErr("");
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
          setErr(formatPublicError(e));
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
  }, [live, paused]);

  const connLabel =
    conn === "connecting"
      ? t("logs.connecting")
      : conn === "live"
        ? t("logs.connected")
        : conn === "paused"
          ? t("logs.paused")
          : conn === "reconnect"
            ? t("logs.reconnectIn", { s: retryIn || 1 })
            : conn === "error"
              ? t("common.error")
              : t("logs.stop");

  const empty =
    !live
      ? t("logs.startHint")
      : shown.length === 0 && rows.length > 0
        ? t("logs.filterEmpty")
        : conn === "live"
          ? t("logs.waiting")
          : t("logs.empty");

  return (
    <div style={{ padding: "14px 22px 40px", display: "flex", flexDirection: "column", gap: 14 }}>
      <SectionHeader
        icon="list"
        title={t("logs.title")}
        description={t("logs.desc")}
        actions={
          <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
            <Button
              variant={live ? "accent" : "secondary"}
              onClick={() => {
                setLive((v) => !v);
                if (live) setPaused(false);
              }}
            >
              {live ? t("logs.stop") : t("logs.start")}
            </Button>
            <Button disabled={!live} onClick={() => setPaused((v) => !v)}>
              {paused ? t("logs.resume") : t("logs.pause")}
            </Button>
            <Button
              variant="quiet"
              disabled={!live}
              onClick={() => {
                seedGen.current += 1;
                setRows([]);
              }}
            >
              {t("logs.clear")}
            </Button>
          </div>
        }
      />
      {live ? (
        <p role="status" style={{ margin: 0, fontSize: 12.5, color: conn === "error" ? "var(--red)" : "var(--text-3)" }}>
          {connLabel}
        </p>
      ) : null}
      {err ? <StatusLine kind="error">{err}</StatusLine> : null}
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "center" }}>
        <select className="z-field" value={component} onChange={(e) => setComponent(e.target.value)} aria-label={t("logs.filter.component")}>
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
                aria-pressed={on}
                onClick={() =>
                  setLevels((prev) => {
                    const next = toggleLevel(prev, lv);
                    return next.length === 0 ? prev : next;
                  })
                }
              >
                {t(LEVEL_KEYS[lv])}
              </Button>
            );
          })}
        </div>
      </div>
      <Card>
        <CardHeader icon="list" title={t("logs.list")} meta={live ? `${connLabel} · ${t("logs.meta", { n: shown.length })}` : t("logs.meta", { n: shown.length })} />
        <TableScroll>
          <div style={{ display: "flex", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)" }}>
            <span style={{ flex: 1.4 }}>{t("logs.col.ts")}</span>
            <span style={{ flex: 0.8 }}>{t("logs.col.level")}</span>
            <span style={{ flex: 1 }}>{t("logs.col.component")}</span>
            <span style={{ flex: 3.2 }}>{t("logs.col.message")}</span>
          </div>
          {loading && shown.length === 0 ? (
            <StatusLine kind="loading">{t("logs.connecting")}</StatusLine>
          ) : shown.length === 0 ? (
            <EmptyState>{empty}</EmptyState>
          ) : (
            shown.map((e, i) => (
              <div
                key={typeof e.seq === "number" ? `seq:${e.seq}` : `${e.ts}:${i}`}
                style={{ display: "flex", alignItems: "center", padding: "11px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 12.5 }}
              >
                <span style={{ flex: 1.4, color: "var(--text-3)", fontVariantNumeric: "tabular-nums" }}>{e.ts}</span>
                <span style={{ flex: 0.8 }}>
                  <Badge tone={levelTone(e.level)}>{e.level.toUpperCase()}</Badge>
                </span>
                <span style={{ flex: 1, color: "var(--text-2)" }}>{e.component}</span>
                <span style={{ flex: 3.2, color: "var(--text-2)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", fontFamily: "var(--font-mono, inherit)" }}>
                  {e.message}
                </span>
              </div>
            ))
          )}
        </TableScroll>
      </Card>
    </div>
  );
}
