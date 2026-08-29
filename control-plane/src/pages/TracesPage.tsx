import { useEffect, useMemo, useState } from "react";
import { tracesApi, type TraceDetail, type TraceItem, type TraceSpan } from "../api/traces";
import {
  PAGE_SIZE,
  childrenOf,
  pageLabel,
  parseTraceHash,
  publicHasSecrets,
  rangeFrom,
  spanRoots,
  statusOf,
  tokenTotal,
  tracesHash,
  uniqueValues,
  type TimeRange,
} from "../api/traces-ops";
import { useI18n } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { SectionHeader } from "../ui/SectionHeader";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

function SpanNode({ span, spans, depth }: { span: TraceSpan; spans: TraceSpan[]; depth: number }) {
  const kids = childrenOf(spans, span.span_id || "");
  const err = statusOf(span) === "error";
  return (
    <div style={{ marginLeft: depth * 14, padding: "6px 0", fontSize: 12.5, borderBottom: "1px solid var(--border-soft)" }}>
      <div style={{ display: "flex", gap: 8, alignItems: "center", flexWrap: "wrap" }}>
        <Badge tone={err ? "critical" : span.kind === "llm" ? "accent" : span.kind === "tool" ? "warning" : "neutral"}>
          {span.kind || "span"}
        </Badge>
        <span style={{ fontWeight: 600 }}>{span.name || span.span_id || "—"}</span>
        <span style={{ color: "var(--text-3)", fontVariantNumeric: "tabular-nums" }}>{span.latency_ms ?? "—"}ms</span>
        {span.status ? <span style={{ color: "var(--text-2)" }}>{span.status}</span> : null}
        {span.input_tokens || span.output_tokens ? (
          <span style={{ color: "var(--text-3)", fontVariantNumeric: "tabular-nums" }}>
            {span.input_tokens ?? 0}→{span.output_tokens ?? 0}
          </span>
        ) : null}
      </div>
      {span.error ? <div style={{ color: "var(--red)", marginTop: 2 }}>{span.error}</div> : null}
      {kids.map((k, i) => (
        <SpanNode key={k.span_id || `${depth}-${i}`} span={k} spans={spans} depth={depth + 1} />
      ))}
    </div>
  );
}

function writeHash(id: string) {
  const next = tracesHash(id);
  if (typeof window === "undefined") return;
  if (window.location.hash === next) return;
  window.history.replaceState(null, "", `${window.location.pathname}${window.location.search}${next}`);
}

export function TracesPage() {
  const { t } = useI18n();
  const [items, setItems] = useState<TraceItem[]>([]);
  const [total, setTotal] = useState(0);
  const [offset, setOffset] = useState(0);
  const [truncated, setTruncated] = useState(false);
  const [groups, setGroups] = useState<{ message: string; count: number }[]>([]);
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(true);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detail, setDetail] = useState<TraceDetail | null>(null);
  const [detailErr, setDetailErr] = useState("");
  const [q, setQ] = useState("");
  const [appliedQ, setAppliedQ] = useState("");
  const [agent, setAgent] = useState("");
  const [channel, setChannel] = useState("");
  const [status, setStatus] = useState("");
  const [range, setRange] = useState<TimeRange>("all");
  const [selectedId, setSelectedId] = useState(() => (typeof window === "undefined" ? "" : parseTraceHash(window.location.hash)));

  const agents = useMemo(() => uniqueValues(items, "agent_id"), [items]);
  const channels = useMemo(() => uniqueValues(items, "channel"), [items]);
  const showChannel = channels.length > 0 || Boolean(channel);
  const filterEmpty = !loading && total === 0 && (Boolean(appliedQ) || Boolean(agent) || Boolean(channel) || Boolean(status) || range !== "all");
  const page = pageLabel(offset, PAGE_SIZE, total);

  async function load(nextOffset = offset) {
    setLoading(true);
    try {
      const j = await tracesApi.list({
        q: appliedQ || undefined,
        agent: agent || undefined,
        channel: channel || undefined,
        status: status || undefined,
        from: rangeFrom(range),
        limit: PAGE_SIZE,
        offset: nextOffset,
      });
      setItems(j.items);
      setTotal(j.total);
      setOffset(j.offset);
      setTruncated(j.truncated);
      setGroups(j.error_groups);
      setErr("");
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setLoading(false);
    }
  }

  async function openTrace(id: string) {
    if (!id) {
      setDetail(null);
      setSelectedId("");
      writeHash("");
      return;
    }
    setSelectedId(id);
    writeHash(id);
    setDetailLoading(true);
    try {
      const d = await tracesApi.get(id);
      setDetail(d);
      setDetailErr("");
    } catch (e) {
      setDetail(null);
      setDetailErr(formatPublicError(e));
    } finally {
      setDetailLoading(false);
    }
  }

  useEffect(() => {
    void load(0);
  }, [appliedQ, agent, channel, status, range]);

  useEffect(() => {
    if (selectedId) void openTrace(selectedId);
    const onHash = () => {
      const id = parseTraceHash(window.location.hash);
      setSelectedId(id);
      if (id) void openTrace(id);
    };
    window.addEventListener("hashchange", onHash);
    return () => {
      window.removeEventListener("hashchange", onHash);
      if (window.location.hash.startsWith("#traces")) {
        window.history.replaceState(null, "", `${window.location.pathname}${window.location.search}`);
      }
    };
  }, []);

  const summary = detail?.item || items.find((it) => it.trace_id === selectedId) || null;
  const spans = detail?.spans || [];
  const roots = spanRoots(spans);
  const leak = publicHasSecrets(detail) || items.some((it) => publicHasSecrets(it));

  return (
    <div style={{ padding: "14px 22px 40px", display: "flex", flexDirection: "column", gap: 14 }}>
      <SectionHeader
        icon="history"
        title={t("traces.title")}
        description={t("traces.desc")}
        actions={
          <Button icon="refresh" iconGesture onClick={() => void load(offset)}>
            {t("common.refresh")}
          </Button>
        }
      />
      {err ? <StatusLine kind="error">{err}</StatusLine> : null}
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "center" }}>
        <input
          className="z-field"
          style={{ flex: 1, minWidth: 160 }}
          placeholder={t("traces.search")}
          value={q}
          onChange={(e) => setQ(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") setAppliedQ(q.trim());
          }}
          aria-label={t("traces.search")}
          autoComplete="off"
        />
        <Button
          icon="search"
          onClick={() => {
            setAppliedQ(q.trim());
          }}
        >
          {t("common.search")}
        </Button>
        <select className="z-field" value={status} onChange={(e) => setStatus(e.target.value)} aria-label={t("traces.filter.status")}>
          <option value="">{t("traces.filter.statusAll")}</option>
          <option value="ok">{t("traces.status.ok")}</option>
          <option value="error">{t("traces.status.error")}</option>
        </select>
        <select className="z-field" value={agent} onChange={(e) => setAgent(e.target.value)} aria-label={t("traces.filter.agent")}>
          <option value="">{t("traces.filter.agentAll")}</option>
          {agents.map((a) => (
            <option key={a} value={a}>
              {a}
            </option>
          ))}
        </select>
        {showChannel ? (
          <select className="z-field" value={channel} onChange={(e) => setChannel(e.target.value)} aria-label={t("traces.filter.channel")}>
            <option value="">{t("traces.filter.channelAll")}</option>
            {channels.map((c) => (
              <option key={c} value={c}>
                {c}
              </option>
            ))}
          </select>
        ) : null}
        <select
          className="z-field"
          value={range}
          onChange={(e) => setRange(e.target.value as TimeRange)}
          aria-label={t("traces.filter.range")}
        >
          <option value="15m">{t("traces.range.15m")}</option>
          <option value="1h">{t("traces.range.1h")}</option>
          <option value="24h">{t("traces.range.24h")}</option>
          <option value="7d">{t("traces.range.7d")}</option>
          <option value="all">{t("traces.range.all")}</option>
        </select>
      </div>
      {groups.length ? (
        <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
          {groups.map((g) => (
            <button
              key={g.message}
              type="button"
              className="z-field"
              onClick={() => {
                setQ(g.message);
                setAppliedQ(g.message);
                setStatus("error");
              }}
              aria-label={t("traces.errorGroup", { n: g.count })}
              style={{ cursor: "pointer" }}
            >
              <Badge tone="critical">{g.count}</Badge> {g.message}
            </button>
          ))}
        </div>
      ) : null}
      <Card>
        <CardHeader icon="history" title={t("traces.list")} meta={t("traces.meta", { n: total })} />
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
              gap: 8,
            }}
          >
            <span style={{ flex: 1.4 }}>{t("traces.col.ts")}</span>
            <span style={{ flex: 0.8 }}>{t("traces.col.status")}</span>
            <span style={{ flex: 1 }}>{t("traces.col.agent")}</span>
            {showChannel ? <span style={{ flex: 0.9 }}>{t("traces.col.channel")}</span> : null}
            <span style={{ flex: 0.7 }}>{t("traces.col.latency")}</span>
            <span style={{ flex: 0.7 }}>{t("traces.col.tokens")}</span>
            <span style={{ flex: 1.6 }}>{t("traces.col.error")}</span>
          </div>
          {items.map((it) => {
            const on = selectedId === it.trace_id;
            const bad = statusOf(it) === "error";
            return (
              <button
                key={it.trace_id}
                type="button"
                onClick={() => void openTrace(it.trace_id)}
                aria-current={on ? "true" : undefined}
                style={{
                  display: "flex",
                  width: "100%",
                  textAlign: "left",
                  padding: "11px 16px",
                  fontSize: 12.5,
                  border: "none",
                  borderBottom: "1px solid var(--border-soft)",
                  cursor: "pointer",
                  background: on ? "var(--accent-soft)" : "transparent",
                  color: "var(--text)",
                  gap: 8,
                  alignItems: "center",
                }}
              >
                <span style={{ flex: 1.4, color: "var(--text-3)", fontVariantNumeric: "tabular-nums" }}>{it.ts || "—"}</span>
                <span style={{ flex: 0.8 }}>
                  <Badge tone={bad ? "critical" : "positive"}>{bad ? t("traces.status.error") : t("traces.status.ok")}</Badge>
                </span>
                <span style={{ flex: 1, fontWeight: 600 }}>{it.agent_id || it.provider || "—"}</span>
                {showChannel ? <span style={{ flex: 0.9, color: "var(--text-2)" }}>{it.channel || "—"}</span> : null}
                <span style={{ flex: 0.7, fontVariantNumeric: "tabular-nums" }}>{it.latency_ms ?? "—"}</span>
                <span style={{ flex: 0.7, fontVariantNumeric: "tabular-nums" }}>{tokenTotal(it) || "—"}</span>
                <span style={{ flex: 1.6, color: "var(--red)" }}>{it.error || ""}</span>
              </button>
            );
          })}
          {loading ? <StatusLine kind="loading" /> : null}
          {!loading && items.length === 0 ? (
            <EmptyState>{filterEmpty ? t("traces.filterEmpty") : t("traces.empty")}</EmptyState>
          ) : null}
        </TableScroll>
        <div style={{ display: "flex", gap: 8, alignItems: "center", padding: "10px 16px", flexWrap: "wrap" }}>
          <Button disabled={loading || offset <= 0} onClick={() => void load(Math.max(0, offset - PAGE_SIZE))}>
            {t("traces.prev")}
          </Button>
          <span style={{ fontSize: 12.5, color: "var(--text-3)" }}>{t("traces.page", { from: page.from, to: page.to, n: total })}</span>
          <Button disabled={loading || offset + PAGE_SIZE >= total} onClick={() => void load(offset + PAGE_SIZE)}>
            {t("traces.next")}
          </Button>
          {truncated ? <span style={{ fontSize: 12, color: "var(--text-3)" }}>{t("traces.truncated")}</span> : null}
        </div>
      </Card>
      <Card>
        <CardHeader icon="layers" title={t("traces.detail")} meta={selectedId || t("traces.emptyDetail")} />
        {detailErr ? <StatusLine kind="error">{detailErr}</StatusLine> : null}
        {detailLoading ? <StatusLine kind="loading" /> : null}
        {!detailLoading && !selectedId ? <EmptyState>{t("traces.emptyDetail")}</EmptyState> : null}
        {!detailLoading && selectedId && !detail && !detailErr ? <EmptyState>{t("traces.partial")}</EmptyState> : null}
        {summary ? (
          <div style={{ display: "flex", gap: 8, flexWrap: "wrap", padding: "10px 16px" }}>
            <Badge tone={statusOf(summary) === "error" ? "critical" : "positive"}>
              {statusOf(summary) === "error" ? t("traces.status.error") : t("traces.status.ok")}
            </Badge>
            <Badge tone="neutral">{t("traces.kpi.latency", { n: summary.latency_ms ?? 0 })}</Badge>
            <Badge tone="neutral">{t("traces.kpi.tokens", { in: summary.input_tokens ?? 0, out: summary.output_tokens ?? 0 })}</Badge>
            {summary.cache_read_tokens ? <Badge tone="accent">{t("traces.kpi.cache", { n: summary.cache_read_tokens })}</Badge> : null}
            {summary.agent_id ? <Badge tone="neutral">{t("traces.kpi.agent", { id: summary.agent_id })}</Badge> : null}
            {summary.channel ? <Badge tone="neutral">{t("traces.kpi.channel", { id: summary.channel })}</Badge> : null}
            {detail?.truncated ? <Badge tone="warning">{t("traces.bounded")}</Badge> : null}
            {leak ? <Badge tone="critical">{t("traces.redacted")}</Badge> : null}
          </div>
        ) : null}
        {spans.length ? (
          <div style={{ padding: "0 16px 12px" }}>
            {roots.map((s, i) => (
              <SpanNode key={s.span_id || i} span={s} spans={spans} depth={0} />
            ))}
          </div>
        ) : selectedId && !detailLoading && detail ? (
          <EmptyState>{t("traces.emptySpans")}</EmptyState>
        ) : null}
      </Card>
    </div>
  );
}
