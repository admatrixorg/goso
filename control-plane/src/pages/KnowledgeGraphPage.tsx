import { useEffect, useState } from "react";
import { api, type Agent } from "../api/client";
import {
  EDGE_CAP,
  NODE_CAP,
  formatWhen,
  isEmbeddingConfigured,
  isInferred,
  kgApi,
  kgSnippet,
  normalizeScope,
  plainKgBody,
  publicHasSecrets,
  type KgExpand,
  type KgGraph,
  type KgIndex,
  type KgNodeLite,
} from "../api/kg";
import { useI18n } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { SectionHeader } from "../ui/SectionHeader";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

type ViewMode = "list" | "graph";

export function KnowledgeGraphPage() {
  const { t } = useI18n();
  const [agents, setAgents] = useState<Agent[]>([]);
  const [agentId, setAgentId] = useState("");
  const [scope, setScope] = useState("");
  const [q, setQ] = useState("");
  const [mode, setMode] = useState<ViewMode>("list");
  const [graph, setGraph] = useState<KgGraph | null>(null);
  const [index, setIndex] = useState<KgIndex | null>(null);
  const [expand, setExpand] = useState<KgExpand | null>(null);
  const [selected, setSelected] = useState("");
  const [loading, setLoading] = useState(false);
  const [agentsLoading, setAgentsLoading] = useState(true);
  const [detailLoading, setDetailLoading] = useState(false);
  const [err, setErr] = useState("");
  const na = t("kg.na");
  const ready = Boolean(agentId && normalizeScope(scope));
  const embedOn = isEmbeddingConfigured(index || graph);
  const leak = Boolean(
    graph && (graph.nodes.some((n) => publicHasSecrets(n)) || graph.edges.some((e) => publicHasSecrets(e))),
  );

  function agentLabel(id: string): string {
    const a = agents.find((x) => x.id === id || x.agent_key === id);
    if (!a) return id;
    return (a.display_name || "").trim() || a.agent_key || id;
  }

  function sourceLabel(row: { source?: string; inferred?: boolean }): string {
    if (isInferred(row)) return t("kg.source.extracted");
    return t("kg.source.posted");
  }

  async function loadAgents() {
    setAgentsLoading(true);
    try {
      const ag = await api.listAgents();
      setAgents(ag.agents ?? []);
      setErr("");
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setAgentsLoading(false);
    }
    try {
      setIndex(await kgApi.index());
    } catch {
      setIndex({ search: "substring", fts: false, embedding: "not_configured", embedding_configured: false });
    }
  }

  async function loadGraph() {
    if (!agentId || !normalizeScope(scope)) {
      setGraph(null);
      setExpand(null);
      setSelected("");
      return;
    }
    setLoading(true);
    try {
      const g = await kgApi.graph({
        agent_id: agentId,
        scope,
        q: q.trim() || undefined,
        limit: NODE_CAP,
      });
      setGraph(g);
      setErr(g.nodes.some((n) => publicHasSecrets(n)) ? t("kg.leak") : "");
    } catch (e) {
      setErr(formatPublicError(e));
      setGraph(null);
    } finally {
      setLoading(false);
    }
  }

  async function openNode(id: string) {
    setSelected(id);
    setDetailLoading(true);
    try {
      setExpand(await kgApi.expand(id));
      setErr("");
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setDetailLoading(false);
    }
  }

  useEffect(() => {
    void loadAgents();
  }, []);

  useEffect(() => {
    if (!ready) {
      setGraph(null);
      setExpand(null);
      setSelected("");
      setLoading(false);
      return;
    }
    void loadGraph();
  }, [agentId, scope]);

  const nodes = graph?.nodes ?? [];
  const edges = graph?.edges ?? [];
  const selectedNode: KgNodeLite | undefined = nodes.find((n) => n.id === selected);
  const bodyText = expand?.entity ? plainKgBody(expand.entity.body) : "";

  return (
    <div style={{ padding: "14px 22px 40px", display: "flex", flexDirection: "column", gap: 14 }}>
      <SectionHeader
        icon="sitemap"
        title={t("kg.title")}
        description={t("kg.desc")}
        actions={
          <Button icon="refresh" iconGesture onClick={() => (ready ? void loadGraph() : void loadAgents())}>
            {t("common.refresh")}
          </Button>
        }
      />
      {err ? <StatusLine kind="error">{err}</StatusLine> : null}
      {leak ? <StatusLine kind="error">{t("kg.leak")}</StatusLine> : null}
      <p role="status" style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>
        {index?.fts || graph?.fts ? t("kg.index.fts") : t("kg.index.substring")}{" "}
        {!embedOn ? (
          <>
            <Badge tone="warning">{t("kg.index.embedOff")}</Badge> {t("kg.index.embedGuide")}
          </>
        ) : null}
      </p>
      <p role="note" style={{ margin: 0, fontSize: 12.5, color: "var(--text-2)" }}>
        {t("kg.notFacts")}
      </p>
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "center" }}>
        <select className="z-field" value={agentId} onChange={(e) => setAgentId(e.target.value)} aria-label={t("kg.agent")} style={{ minWidth: 160 }}>
          <option value="">{t("kg.pickAgent")}</option>
          {agents.map((a) => (
            <option key={a.id} value={a.id}>
              {agentLabel(a.id)}
            </option>
          ))}
        </select>
        <select className="z-field" value={scope} onChange={(e) => setScope(e.target.value)} aria-label={t("kg.scope")} style={{ minWidth: 160 }}>
          <option value="">{t("kg.pickScope")}</option>
          <option value="all">{t("kg.scope.all")}</option>
          <option value="posted">{t("kg.scope.posted")}</option>
          <option value="extracted">{t("kg.scope.extracted")}</option>
        </select>
        <input
          className="z-field"
          style={{ flex: 1, minWidth: 160 }}
          placeholder={t("kg.search")}
          value={q}
          onChange={(e) => setQ(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && ready) void loadGraph();
          }}
          aria-label={t("kg.search")}
          autoComplete="off"
          disabled={!ready}
        />
        <Button icon="search" onClick={() => void loadGraph()} disabled={!ready}>
          {t("common.search")}
        </Button>
        <select className="z-field" value={mode} onChange={(e) => setMode(e.target.value as ViewMode)} aria-label={t("kg.view")} style={{ minWidth: 140 }} disabled={!ready}>
          <option value="list">{t("kg.view.list")}</option>
          <option value="graph">{t("kg.view.graph")}</option>
        </select>
      </div>
      {agentsLoading ? <StatusLine kind="loading" /> : null}
      {!agentsLoading && agents.length === 0 ? <EmptyState>{t("kg.emptyAgents")}</EmptyState> : null}
      {!agentsLoading && agents.length > 0 && !ready ? <EmptyState>{t("kg.needSelect")}</EmptyState> : null}
      {ready && loading ? <StatusLine kind="loading" /> : null}
      {ready && !loading && graph && nodes.length === 0 ? <EmptyState>{t("kg.empty")}</EmptyState> : null}
      {ready && graph ? (
        <div className="z-two-col">
          <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
            <Card>
              <CardHeader
                icon="sitemap"
                title={mode === "graph" ? t("kg.graph") : t("kg.list")}
                meta={t("kg.meta", { nodes: String(graph.total_nodes ?? nodes.length), edges: String(graph.total_edges ?? edges.length) })}
              />
              <div style={{ padding: "8px 16px 0", fontSize: 12, color: "var(--text-3)" }}>{t("kg.noCanvas")}</div>
              {graph.truncated ? (
                <p style={{ margin: "8px 16px 0", fontSize: 12, color: "var(--orange)" }}>{t("kg.truncated", { n: String(graph.node_cap || NODE_CAP) })}</p>
              ) : null}
              <TableScroll>
                <div style={{ display: "flex", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)", gap: 8 }}>
                  <span style={{ flex: 2 }}>{t("kg.col.name")}</span>
                  <span style={{ flex: 1 }}>{t("kg.col.kind")}</span>
                  <span style={{ flex: 1 }}>{t("kg.col.source")}</span>
                  <span style={{ flex: 2 }}>{t("kg.col.snippet")}</span>
                </div>
                {nodes.map((n) => {
                  const on = selected === n.id;
                  return (
                    <button
                      key={n.id}
                      type="button"
                      onClick={() => void openNode(n.id)}
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
                      <span style={{ flex: 2, fontWeight: 600 }}>{n.name}</span>
                      <span style={{ flex: 1, color: "var(--text-2)" }}>{n.kind || "—"}</span>
                      <span style={{ flex: 1 }}>
                        {isInferred(n) ? <Badge tone="warning">{sourceLabel(n)}</Badge> : sourceLabel(n)}
                      </span>
                      <span style={{ flex: 2, color: "var(--text-2)" }}>{kgSnippet(n.snippet || n.name)}</span>
                    </button>
                  );
                })}
              </TableScroll>
            </Card>
            {mode === "graph" ? (
              <Card>
                <CardHeader icon="hook" title={t("kg.edges")} meta={t("kg.edgeMeta", { n: String(edges.length), cap: String(graph.edge_cap || EDGE_CAP) })} />
                <TableScroll>
                  <div style={{ display: "flex", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)", gap: 8 }}>
                    <span style={{ flex: 2 }}>{t("kg.col.from")}</span>
                    <span style={{ flex: 1 }}>{t("kg.col.rel")}</span>
                    <span style={{ flex: 2 }}>{t("kg.col.to")}</span>
                    <span style={{ flex: 1 }}>{t("kg.col.source")}</span>
                  </div>
                  {edges.map((e) => (
                    <div key={e.id} style={{ display: "flex", padding: "11px 16px", fontSize: 12.5, borderBottom: "1px solid var(--border-soft)", gap: 8 }}>
                      <span style={{ flex: 2 }}>{e.from_name || e.from_id}</span>
                      <span style={{ flex: 1 }}>{e.rel}</span>
                      <span style={{ flex: 2 }}>{e.to_name || e.to_id}</span>
                      <span style={{ flex: 1 }}>{isInferred(e) ? <Badge tone="warning">{sourceLabel(e)}</Badge> : sourceLabel(e)}</span>
                    </div>
                  ))}
                  {edges.length === 0 ? <EmptyState>{t("kg.emptyEdges")}</EmptyState> : null}
                </TableScroll>
              </Card>
            ) : null}
          </div>
          <Card>
            <CardHeader icon="inbox" title={t("kg.detail")} meta={selectedNode?.name || t("kg.emptyDetail")} />
            <div style={{ padding: "10px 16px", fontSize: 12.5 }}>
              {detailLoading ? <StatusLine kind="loading" /> : null}
              {!detailLoading && !expand?.entity ? <EmptyState style={{ padding: "24px 8px" }}>{t("kg.emptyDetail")}</EmptyState> : null}
              {expand?.entity && !detailLoading ? (
                <>
                  <dl style={{ display: "grid", gridTemplateColumns: "auto 1fr", gap: "4px 12px", margin: "0 0 12px" }}>
                    <dt style={{ color: "var(--text-3)" }}>{t("kg.metaId")}</dt>
                    <dd style={{ margin: 0 }}>{expand.entity.id}</dd>
                    <dt style={{ color: "var(--text-3)" }}>{t("kg.metaKind")}</dt>
                    <dd style={{ margin: 0 }}>{expand.entity.kind}</dd>
                    <dt style={{ color: "var(--text-3)" }}>{t("kg.metaAgent")}</dt>
                    <dd style={{ margin: 0 }}>{expand.entity.agent_id ? agentLabel(expand.entity.agent_id) : agentLabel(agentId)}</dd>
                    <dt style={{ color: "var(--text-3)" }}>{t("kg.metaSource")}</dt>
                    <dd style={{ margin: 0 }}>{sourceLabel({ source: expand.entity.source, inferred: isInferred({ source: expand.entity.source }) })}</dd>
                    <dt style={{ color: "var(--text-3)" }}>{t("kg.metaCreated")}</dt>
                    <dd style={{ margin: 0 }}>{formatWhen(expand.entity.created_at, na)}</dd>
                    <dt style={{ color: "var(--text-3)" }}>{t("kg.metaValid")}</dt>
                    <dd style={{ margin: 0 }}>
                      {formatWhen(expand.entity.valid_from, na)}
                      {expand.entity.valid_until ? ` → ${formatWhen(expand.entity.valid_until, na)}` : ""}
                    </dd>
                  </dl>
                  {isInferred({ source: expand.entity.source }) ? (
                    <p style={{ margin: "0 0 8px", fontSize: 12, color: "var(--orange)" }}>{t("kg.inferredNote")}</p>
                  ) : null}
                  {bodyText ? <pre style={{ margin: "0 0 8px", whiteSpace: "pre-wrap", fontSize: 12, color: "var(--text-2)", fontFamily: "inherit" }}>{bodyText}</pre> : null}
                  <div style={{ fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)", marginBottom: 6 }}>{t("kg.relations")}</div>
                  {(expand.relations ?? []).length === 0 ? (
                    <EmptyState>{t("kg.emptyEdges")}</EmptyState>
                  ) : (
                    (expand.relations ?? []).map((rel) => (
                      <div key={rel.id} style={{ padding: "6px 0", borderBottom: "1px solid var(--border-soft)", color: "var(--text-2)" }}>
                        {rel.from_name || rel.from_id} — {rel.rel} → {rel.to_name || rel.to_id}
                        {isInferred(rel) ? ` · ${t("kg.source.extracted")}` : ""}
                      </div>
                    ))
                  )}
                </>
              ) : null}
            </div>
          </Card>
        </div>
      ) : null}
    </div>
  );
}
