import { useEffect, useState } from "react";
import { tracesApi, type LlmTrace, type SpanTree, type TraceSpan } from "../api/traces";
import { useI18n } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { SectionHeader } from "../ui/SectionHeader";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

function childrenOf(spans: TraceSpan[], parentId: string): TraceSpan[] {
  return spans.filter((s) => (s.parent_id || "") === parentId);
}

function SpanNode({ span, spans, depth }: { span: TraceSpan; spans: TraceSpan[]; depth: number }) {
  const kids = childrenOf(spans, span.span_id || "");
  return (
    <div style={{ marginLeft: depth * 14, padding: "6px 0", fontSize: 12.5, borderBottom: "1px solid var(--border-soft)" }}>
      <div style={{ display: "flex", gap: 8, alignItems: "center", flexWrap: "wrap" }}>
        <Badge tone={span.error ? "critical" : span.kind === "llm" ? "accent" : "neutral"}>{span.kind || "span"}</Badge>
        <span style={{ fontWeight: 600 }}>{span.name || span.span_id || "—"}</span>
        <span style={{ color: "var(--text-3)", fontVariantNumeric: "tabular-nums" }}>{span.latency_ms ?? "—"}ms</span>
        {span.status ? <span style={{ color: "var(--text-2)" }}>{span.status}</span> : null}
      </div>
      {span.error ? <div style={{ color: "var(--red)", marginTop: 2 }}>{span.error}</div> : null}
      {kids.map((k, i) => (
        <SpanNode key={k.span_id || `${depth}-${i}`} span={k} spans={spans} depth={depth + 1} />
      ))}
    </div>
  );
}

function SpanTreeCard({ tree, i }: { tree: SpanTree; i: number }) {
  const spans = Array.isArray(tree.spans) ? tree.spans : [];
  const roots = spans.filter((s) => !s.parent_id);
  const fallback = roots.length ? roots : spans.slice(0, 1);
  return (
    <div style={{ padding: "10px 16px", borderBottom: "1px solid var(--border-soft)" }}>
      <div style={{ fontSize: 11.5, color: "var(--text-3)", marginBottom: 6 }}>{tree.trace_id || `tree-${i}`}</div>
      {fallback.map((s, j) => (
        <SpanNode key={s.span_id || `${i}-${j}`} span={s} spans={spans} depth={0} />
      ))}
      {spans.length === 0 ? (
        <pre style={{ margin: 0, fontSize: 11.5, whiteSpace: "pre-wrap", color: "var(--text-3)" }}>
          {safeJson(tree)}
        </pre>
      ) : null}
    </div>
  );
}

function safeJson(v: unknown): string {
  try {
    return JSON.stringify(v, null, 2);
  } catch {
    return String(v);
  }
}

export function TracesPage() {
  const { t } = useI18n();
  const [traces, setTraces] = useState<LlmTrace[]>([]);
  const [spans, setSpans] = useState<SpanTree[]>([]);
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(true);

  async function load() {
    try {
      const j = await tracesApi.list(50);
      setTraces(j.traces ?? []);
      setSpans(j.spans ?? []);
      setErr("");
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setLoading(false);
    }
  }
  useEffect(() => {
    void load();
  }, []);

  return (
    <div style={{ padding: "14px 22px 40px", display: "flex", flexDirection: "column", gap: 14 }}>
      <SectionHeader
        icon="history"
        title={t("traces.title")}
        description={t("traces.desc")}
        actions={
          <Button icon="refresh" iconGesture onClick={() => void load()}>
            {t("common.refresh")}
          </Button>
        }
      />
      {err ? <StatusLine kind="error">{err}</StatusLine> : null}
      <Card>
        <CardHeader icon="history" title={t("traces.list")} meta={t("traces.meta", { n: traces.length })} />
        <div style={{ display: "flex", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)" }}>
          <span style={{ flex: 1.6 }}>{t("traces.col.ts")}</span>
          <span style={{ flex: 1.1 }}>{t("traces.col.provider")}</span>
          <span style={{ flex: 1.1 }}>{t("traces.col.model")}</span>
          <span style={{ flex: 0.7 }}>{t("traces.col.latency")}</span>
          <span style={{ flex: 2 }}>{t("traces.col.error")}</span>
        </div>
        {traces.map((tr, i) => (
          <div key={`${tr.request_id || tr.ts || i}`} style={{ display: "flex", padding: "11px 16px", fontSize: 12.5, borderBottom: "1px solid var(--border-soft)" }}>
            <span style={{ flex: 1.6, color: "var(--text-3)", fontVariantNumeric: "tabular-nums" }}>{tr.ts || "—"}</span>
            <span style={{ flex: 1.1, fontWeight: 600 }}>{tr.provider || "—"}</span>
            <span style={{ flex: 1.1, color: "var(--text-2)" }}>{tr.model || "—"}</span>
            <span style={{ flex: 0.7, fontVariantNumeric: "tabular-nums" }}>{tr.latency_ms ?? "—"}</span>
            <span style={{ flex: 2, color: "var(--red)" }}>{tr.error || ""}</span>
          </div>
        ))}
        {loading ? <StatusLine kind="loading" /> : traces.length === 0 ? <EmptyState>{t("traces.empty")}</EmptyState> : null}
      </Card>
      <Card>
        <CardHeader icon="layers" title={t("traces.spans")} meta={String(spans.length)} />
        {spans.map((tree, i) => (
          <SpanTreeCard key={tree.trace_id || i} tree={tree} i={i} />
        ))}
        {loading ? <StatusLine kind="loading" /> : spans.length === 0 ? <EmptyState>{t("traces.emptySpans")}</EmptyState> : null}
      </Card>
    </div>
  );
}
