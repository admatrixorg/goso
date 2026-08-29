import { useEffect, useMemo, useRef, useState } from "react";
import { eventsApi, type GatewayEvent } from "../api/events";
import {
  applyFilters,
  backoffDelay,
  EVENT_TYPES,
  eventKey,
  mergeLive,
  parseDetail,
  uniqueActors,
  type EventType,
  type StreamConn,
} from "../api/events-ops";
import { useI18n, type MsgKey } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { SectionHeader } from "../ui/SectionHeader";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

function kindTone(k: string): "positive" | "warning" | "critical" | "neutral" | "accent" {
  if (k.includes("error") || k.includes("fail") || k.includes("delete")) return "critical";
  if (k.includes("success") || k.includes("ok") || k.includes("create")) return "positive";
  if (k.includes("pending") || k.includes("approval")) return "warning";
  if (k.includes("attempt") || k.includes("update")) return "accent";
  return "neutral";
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
}: {
  rows: GatewayEvent[];
  open: string;
  onOpen: (key: string) => void;
  empty: string;
}) {
  const { t } = useI18n();
  if (rows.length === 0) return <EmptyState>{empty}</EmptyState>;
  return (
    <>
      {rows.map((e, i) => {
        const key = eventKey(e, i);
        const details = open === key ? parseDetail(e) : [];
        return (
          <div key={key} style={{ borderBottom: "1px solid var(--border-soft)" }}>
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
              <span style={{ flex: 1.4, color: "var(--text-3)", fontVariantNumeric: "tabular-nums" }}>{e.ts}</span>
              <span style={{ flex: 0.9 }}>
                <Badge tone="neutral">{e.type || "connector"}</Badge>
              </span>
              <span style={{ flex: 1.1 }}>
                <Badge tone={kindTone(e.kind)}>{e.kind}</Badge>
              </span>
              <span style={{ flex: 1, color: "var(--text-2)" }}>{e.actor || e.agent_id || e.team_id || "—"}</span>
              <span style={{ flex: 1.1 }}>{e.connector || "—"}</span>
              <span style={{ flex: 0.9, color: "var(--text-2)" }}>{e.tool || e.action || "—"}</span>
              <span style={{ flex: 2.2, color: "var(--text-2)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                {e.summary}
              </span>
            </button>
            {open === key ? (
              <div style={{ padding: "0 16px 12px" }} aria-label={t("events.detail")}>
                {details.length === 0 ? (
                  <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{e.summary || t("events.noDetail")}</p>
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
      })}
    </>
  );
}

export function EventsPage() {
  const { t } = useI18n();
  const [history, setHistory] = useState<GatewayEvent[]>([]);
  const [liveRows, setLiveRows] = useState<GatewayEvent[]>([]);
  const [kind, setKind] = useState("");
  const [connector, setConnector] = useState("");
  const [type, setType] = useState("");
  const [actor, setActor] = useState("");
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(true);
  const [live, setLive] = useState(false);
  const [paused, setPaused] = useState(false);
  const [conn, setConn] = useState<StreamConn>("off");
  const [retryIn, setRetryIn] = useState(0);
  const [open, setOpen] = useState("");
  const lastSeq = useRef(0);

  const filters = useMemo(() => ({ type: type || undefined, actor: actor || undefined, kind: kind || undefined, connector: connector || undefined }), [type, actor, kind, connector]);
  const shownHistory = useMemo(() => applyFilters(history, filters), [history, filters]);
  const shownLive = useMemo(() => applyFilters(liveRows, filters), [liveRows, filters]);
  const actors = useMemo(() => uniqueActors([...liveRows, ...history]), [liveRows, history]);

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
      setErr("");
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    const timer = setTimeout(() => void load(), history.length === 0 ? 0 : 250);
    return () => clearTimeout(timer);
  }, [type, actor, kind, connector]);

  useEffect(() => {
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
      ? t("events.connecting")
      : conn === "live"
        ? t("events.connected")
        : conn === "paused"
          ? t("events.paused")
          : conn === "reconnect"
            ? t("events.reconnectIn", { s: retryIn || 1 })
            : conn === "error"
              ? t("common.error")
              : t("events.liveOff");

  return (
    <div style={{ padding: "14px 22px 40px", display: "flex", flexDirection: "column", gap: 14 }}>
      <SectionHeader
        icon="pulse"
        title={t("events.title")}
        description={t("events.desc")}
        actions={
          <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
            <Button icon="refresh" iconGesture onClick={() => void load()}>
              {t("common.refresh")}
            </Button>
            <Button
              variant={live ? "accent" : "secondary"}
              onClick={() => {
                setLive((v) => !v);
                if (live) setPaused(false);
              }}
            >
              {live ? t("events.liveOn") : t("events.liveOff")}
            </Button>
            <Button
              disabled={!live}
              onClick={() => setPaused((v) => !v)}
            >
              {paused ? t("events.resume") : t("events.pause")}
            </Button>
            <Button
              variant="quiet"
              disabled={!live}
              onClick={() => {
                setLiveRows([]);
                setOpen("");
              }}
            >
              {t("events.clear")}
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
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
        <select className="z-field" value={type} onChange={(e) => setType(e.target.value)} aria-label={t("events.filterType")}>
          <option value="">{t("events.type.all")}</option>
          {EVENT_TYPES.map((tp) => (
            <option key={tp} value={tp}>
              {t(TYPE_KEYS[tp])}
            </option>
          ))}
        </select>
        <input
          className="z-field"
          list="event-actors"
          placeholder={t("events.filterActor")}
          value={actor}
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
          className="z-field"
          placeholder={t("events.filterKind")}
          value={kind}
          onChange={(e) => setKind(e.target.value)}
          aria-label={t("events.filterKind")}
        />
        <input
          className="z-field"
          placeholder={t("events.filterConnector")}
          value={connector}
          onChange={(e) => setConnector(e.target.value)}
          aria-label={t("events.filterConnector")}
        />
      </div>
      {live ? (
        <Card>
          <CardHeader icon="bolt" title={t("events.liveList")} meta={`${connLabel} · ${t("events.meta", { n: shownLive.length })}`} />
          <TableScroll>
            <div style={{ display: "flex", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)" }}>
              <span style={{ flex: 1.4 }}>{t("events.col.ts")}</span>
              <span style={{ flex: 0.9 }}>{t("events.col.type")}</span>
              <span style={{ flex: 1.1 }}>{t("events.col.kind")}</span>
              <span style={{ flex: 1 }}>{t("events.col.actor")}</span>
              <span style={{ flex: 1.1 }}>{t("events.col.connector")}</span>
              <span style={{ flex: 0.9 }}>{t("events.col.tool")}</span>
              <span style={{ flex: 2.2 }}>{t("events.col.summary")}</span>
            </div>
            {conn === "connecting" && shownLive.length === 0 ? (
              <StatusLine kind="loading">{t("events.connecting")}</StatusLine>
            ) : (
              <EventRows rows={shownLive} open={open} onOpen={setOpen} empty={conn === "live" ? t("events.waiting") : t("events.liveEmpty")} />
            )}
          </TableScroll>
        </Card>
      ) : null}
      <Card>
        <CardHeader icon="history" title={t("events.list")} meta={t("events.meta", { n: shownHistory.length })} />
        <TableScroll>
          <div style={{ display: "flex", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)" }}>
            <span style={{ flex: 1.4 }}>{t("events.col.ts")}</span>
            <span style={{ flex: 0.9 }}>{t("events.col.type")}</span>
            <span style={{ flex: 1.1 }}>{t("events.col.kind")}</span>
            <span style={{ flex: 1 }}>{t("events.col.actor")}</span>
            <span style={{ flex: 1.1 }}>{t("events.col.connector")}</span>
            <span style={{ flex: 0.9 }}>{t("events.col.tool")}</span>
            <span style={{ flex: 2.2 }}>{t("events.col.summary")}</span>
          </div>
          {loading ? <StatusLine kind="loading" /> : <EventRows rows={shownHistory} open={open} onOpen={setOpen} empty={t("events.empty")} />}
        </TableScroll>
      </Card>
    </div>
  );
}
