import { useEffect, useMemo, useState } from "react";
import { formatStaleAt, listMetaCount } from "../api/page-state";
import { tracesApi, type TraceDetail, type TraceItem, type TraceSpan } from "../api/traces";
import {
  PAGE_SIZE,
  childrenOf,
  classifyTraceDetail,
  classifyTracesList,
  pageLabel,
  parseTraceHash,
  publicHasSecrets,
  rangeFrom,
  spanRoots,
  statusOf,
  tokenTotal,
  tracesActionsBlocked,
  tracesFilteredEmpty,
  tracesFiltersActive,
  tracesHash,
  tracesTrueEmpty,
  uniqueValues,
  type TimeRange,
} from "../api/traces-ops";
import { useI18n } from "../i18n";
import { Button } from "../ui/Button";
import { Card, CardHeader } from "../ui/Card";
import { DataTable, THead, Th, Td, Tr } from "../ui/DataTable";
import { EmptyState } from "../ui/EmptyState";
import { PageChrome } from "../ui/PageChrome";
import { PageStatus } from "../ui/PageStatus";
import { SearchInput } from "../ui/SearchInput";
import { StatusBadge, type Status } from "../ui/StatusBadge";
import { StatusLine, formatPublicError } from "../ui/StatusLine";
import { selectClass } from "../ui/field";

function spanKindStatus(span: TraceSpan): Status {
  if (statusOf(span) === "error") return "error";
  if (span.kind === "llm") return "info";
  if (span.kind === "tool") return "warning";
  return "default";
}

function SpanNode({ span, spans, depth }: { span: TraceSpan; spans: TraceSpan[]; depth: number }) {
  const kids = childrenOf(spans, span.span_id || "");
  return (
    <div className="border-b py-1.5 text-sm last:border-0" style={{ marginLeft: depth * 14 }}>
      <div className="flex flex-wrap items-center gap-2">
        <StatusBadge status={spanKindStatus(span)} label={span.kind || "span"} />
        <span className="font-semibold">{span.name || span.span_id || "—"}</span>
        <span className="tabular-nums text-muted-foreground">{span.latency_ms ?? "—"}ms</span>
        {span.status ? <span className="text-muted-foreground">{span.status}</span> : null}
        {span.input_tokens || span.output_tokens ? (
          <span className="tabular-nums text-muted-foreground">
            {span.input_tokens ?? 0}→{span.output_tokens ?? 0}
          </span>
        ) : null}
      </div>
      {span.error ? <div className="mt-0.5 text-destructive">{span.error}</div> : null}
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
  const { t, locale } = useI18n();
  const [items, setItems] = useState<TraceItem[]>([]);
  const [total, setTotal] = useState(0);
  const [offset, setOffset] = useState(0);
  const [truncated, setTruncated] = useState(false);
  const [groups, setGroups] = useState<{ message: string; count: number }[]>([]);
  const [err, setErr] = useState<unknown>(null);
  const [leak, setLeak] = useState(false);
  const [loading, setLoading] = useState(true);
  const [loaded, setLoaded] = useState(false);
  const [loadedAt, setLoadedAt] = useState<string | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detail, setDetail] = useState<TraceDetail | null>(null);
  const [detailErr, setDetailErr] = useState<unknown>(null);
  const [q, setQ] = useState("");
  const [appliedQ, setAppliedQ] = useState("");
  const [agent, setAgent] = useState("");
  const [channel, setChannel] = useState("");
  const [status, setStatus] = useState("");
  const [range, setRange] = useState<TimeRange>("all");
  const [selectedId, setSelectedId] = useState(() => (typeof window === "undefined" ? "" : parseTraceHash(window.location.hash)));

  const state = classifyTracesList({ loading, loaded, error: err, itemCount: items.length });
  const blocked = tracesActionsBlocked(state.kind);
  const filtersOn = tracesFiltersActive({ q: appliedQ, agent, channel, status, range });
  const filterEmpty = tracesFilteredEmpty(state, filtersOn);
  const trueEmpty = tracesTrueEmpty(state, filtersOn);
  const agents = useMemo(() => uniqueValues(state.showItems ? items : [], "agent_id"), [items, state.showItems]);
  const channels = useMemo(() => uniqueValues(state.showItems ? items : [], "channel"), [items, state.showItems]);
  const showChannel = channels.length > 0 || Boolean(channel);
  const page = pageLabel(offset, PAGE_SIZE, total);
  const metaN = listMetaCount(state.kind, total);
  const detailKind = classifyTraceDetail({
    selectedId,
    loading: detailLoading,
    error: detailErr,
    hasDetail: Boolean(detail),
  });

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
      setLoaded(true);
      setLoadedAt(new Date().toISOString());
      setErr(null);
      setLeak((j.items || []).some((it) => publicHasSecrets(it)));
    } catch (e) {
      setErr(e);
    } finally {
      setLoading(false);
    }
  }

  async function openTrace(id: string) {
    if (!id) {
      setDetail(null);
      setSelectedId("");
      setDetailErr(null);
      writeHash("");
      return;
    }
    if (blocked) return;
    setSelectedId(id);
    writeHash(id);
    setDetailLoading(true);
    try {
      const d = await tracesApi.get(id);
      setDetail(d);
      setDetailErr(null);
      if (publicHasSecrets(d)) setLeak(true);
    } catch (e) {
      setDetail(null);
      setDetailErr(e);
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
    };
  }, []);

  const summary = detail?.item || (state.showItems ? items.find((it) => it.trace_id === selectedId) : undefined) || null;
  const spans = detail?.spans || [];
  const roots = spanRoots(spans);
  const shownGroups = state.showItems ? groups : [];

  return (
    <PageChrome
      icon="scan"
      title={t("traces.title")}
      description={t("traces.desc")}
      refresh={
        <Button icon="refresh" iconGesture variant="outline" onClick={() => void load(offset)} disabled={loading}>
          {t("common.refresh")}
        </Button>
      }
      filters={
        <div
          className="contents"
          onKeyDown={(e) => {
            if (e.key === "Enter" && !blocked) setAppliedQ(q.trim());
          }}
        >
          <SearchInput
            value={q}
            onChange={(v) => {
              if (!blocked) setQ(v);
            }}
            placeholder={t("traces.search")}
            aria-label={t("traces.search")}
          />
          <Button
            icon="search"
            variant="outline"
            disabled={blocked}
            onClick={() => {
              if (!blocked) setAppliedQ(q.trim());
            }}
          >
            {t("common.search")}
          </Button>
          <select className={selectClass} value={status} disabled={blocked} onChange={(e) => setStatus(e.target.value)} aria-label={t("traces.filter.status")}>
            <option value="">{t("traces.filter.statusAll")}</option>
            <option value="ok">{t("traces.status.ok")}</option>
            <option value="error">{t("traces.status.error")}</option>
          </select>
          <select className={selectClass} value={agent} disabled={blocked} onChange={(e) => setAgent(e.target.value)} aria-label={t("traces.filter.agent")}>
            <option value="">{t("traces.filter.agentAll")}</option>
            {agents.map((a) => (
              <option key={a} value={a}>
                {a}
              </option>
            ))}
          </select>
          {showChannel ? (
            <select className={selectClass} value={channel} disabled={blocked} onChange={(e) => setChannel(e.target.value)} aria-label={t("traces.filter.channel")}>
              <option value="">{t("traces.filter.channelAll")}</option>
              {channels.map((c) => (
                <option key={c} value={c}>
                  {c}
                </option>
              ))}
            </select>
          ) : null}
          <select
            className={selectClass}
            value={range}
            disabled={blocked}
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
      }
    >
      <PageStatus kind={state.kind} errorText={err ? formatPublicError(err) : ""} staleAt={formatStaleAt(loadedAt, locale)} onReload={() => void load(offset)} />
      {leak ? (
        <p role="status" className="text-sm text-warning">
          {t("traces.redacted")}
        </p>
      ) : null}
      {shownGroups.length ? (
        <div role="group" aria-label={t("traces.groups")} className="flex flex-wrap gap-2">
          {shownGroups.map((g) => (
            <Button
              key={g.message}
              variant="outline"
              size="sm"
              disabled={blocked}
              onClick={() => {
                if (blocked) return;
                setQ(g.message);
                setAppliedQ(g.message);
                setStatus("error");
              }}
              aria-label={t("traces.errorGroup", { n: g.count })}
            >
              <StatusBadge status="error" label={String(g.count)} /> {g.message}
            </Button>
          ))}
        </div>
      ) : null}
      <div className="flex items-center">
        <h2 className="text-sm font-semibold">{t("traces.list")}</h2>
        <span className="ml-auto text-xs text-muted-foreground">
          {metaN == null ? "—" : `${t("traces.meta", { n: metaN })}${truncated && state.showItems ? ` · ${t("traces.truncated")}` : ""}`}
        </span>
      </div>
      {trueEmpty ? <EmptyState icon="activity" title={t("traces.empty")} data-page-state="empty" /> : null}
      {filterEmpty ? <EmptyState icon="search" title={t("traces.filterEmpty")} data-page-state="filtered_empty" /> : null}
      {state.showItems ? (
        <DataTable minWidth="800px" className="mt-0">
          <THead>
            <Th>{t("traces.col.ts")}</Th>
            <Th>{t("traces.col.status")}</Th>
            <Th>{t("traces.col.agent")}</Th>
            {showChannel ? <Th>{t("traces.col.channel")}</Th> : null}
            <Th>{t("traces.col.latency")}</Th>
            <Th>{t("traces.col.tokens")}</Th>
            <Th>{t("traces.col.error")}</Th>
          </THead>
          <tbody>
            {items.map((it) => {
              const on = selectedId === it.trace_id;
              const bad = statusOf(it) === "error";
              return (
                <Tr
                  key={it.trace_id}
                  selected={on}
                  aria-current={on ? "true" : undefined}
                  onClick={() => {
                    if (!blocked) void openTrace(it.trace_id);
                  }}
                >
                  <Td className="tabular-nums text-muted-foreground">{it.ts || "—"}</Td>
                  <Td>
                    <StatusBadge status={bad ? "error" : "success"} label={bad ? t("traces.status.error") : t("traces.status.ok")} />
                  </Td>
                  <Td className="font-semibold">{it.agent_id || it.provider || "—"}</Td>
                  {showChannel ? <Td className="text-muted-foreground">{it.channel || "—"}</Td> : null}
                  <Td className="tabular-nums">{it.latency_ms ?? "—"}</Td>
                  <Td className="tabular-nums">{tokenTotal(it) || "—"}</Td>
                  <Td className="text-destructive">{it.error || ""}</Td>
                </Tr>
              );
            })}
          </tbody>
        </DataTable>
      ) : null}
      <div className="flex flex-wrap items-center gap-2">
        <Button variant="outline" size="sm" disabled={blocked || loading || offset <= 0} onClick={() => void load(Math.max(0, offset - PAGE_SIZE))}>
          {t("traces.prev")}
        </Button>
        <span className="text-sm text-muted-foreground">{metaN == null ? "—" : t("traces.page", { from: page.from, to: page.to, n: total })}</span>
        <Button variant="outline" size="sm" disabled={blocked || loading || offset + PAGE_SIZE >= total} onClick={() => void load(offset + PAGE_SIZE)}>
          {t("traces.next")}
        </Button>
        {truncated && state.showItems ? <span className="text-xs text-muted-foreground">{t("traces.truncated")}</span> : null}
      </div>
      <Card>
        <CardHeader icon="layers" title={t("traces.detail")} meta={selectedId || t("traces.emptyDetail")} />
        {detailKind === "loading" ? (
          <div data-page-state="loading">
            <StatusLine kind="loading" />
          </div>
        ) : null}
        {detailKind === "permission" ? (
          <div data-page-state="dependency">
            <StatusLine kind="error">
              {t("traces.detailPermission")}
              {detailErr ? ` · ${formatPublicError(detailErr)}` : ""}
            </StatusLine>
          </div>
        ) : null}
        {detailKind === "error" ? (
          <div data-page-state="dependency">
            <StatusLine kind="error">
              {t("traces.detailFail")}
              {detailErr ? ` · ${formatPublicError(detailErr)}` : ""}
            </StatusLine>
          </div>
        ) : null}
        {detailKind === "idle" ? <EmptyState icon="activity" title={t("traces.emptyDetail")} data-page-state="no_selection" /> : null}
        {detailKind === "missing" ? <EmptyState icon="activity" title={t("traces.missingId")} data-page-state="dependency" /> : null}
        {selectedId ? <p className="px-4 pb-2 text-xs text-muted-foreground">{t("traces.hashHint", { hash: tracesHash(selectedId) })}</p> : null}
        {summary && detailKind !== "idle" && detailKind !== "loading" && detailKind !== "permission" ? (
          <div className="flex flex-wrap gap-2 px-4 py-2.5">
            <StatusBadge
              status={statusOf(summary) === "error" ? "error" : "success"}
              label={statusOf(summary) === "error" ? t("traces.status.error") : t("traces.status.ok")}
            />
            <StatusBadge status="default" label={t("traces.kpi.latency", { n: summary.latency_ms ?? 0 })} />
            <StatusBadge status="default" label={t("traces.kpi.tokens", { in: summary.input_tokens ?? 0, out: summary.output_tokens ?? 0 })} />
            {summary.cache_read_tokens ? <StatusBadge status="info" label={t("traces.kpi.cache", { n: summary.cache_read_tokens })} /> : null}
            {summary.agent_id ? <StatusBadge status="default" label={t("traces.kpi.agent", { id: summary.agent_id })} /> : null}
            {summary.channel ? <StatusBadge status="default" label={t("traces.kpi.channel", { id: summary.channel })} /> : null}
            {detail?.truncated ? <StatusBadge status="warning" label={t("traces.bounded")} /> : null}
            {leak ? <StatusBadge status="error" label={t("traces.redacted")} /> : null}
          </div>
        ) : null}
        {spans.length ? (
          <div className="px-4 pb-3">
            {roots.map((s, i) => (
              <SpanNode key={s.span_id || i} span={s} spans={spans} depth={0} />
            ))}
          </div>
        ) : detailKind === "ready" ? (
          <EmptyState icon="activity" title={t("traces.emptySpans")} />
        ) : null}
      </Card>
    </PageChrome>
  );
}
