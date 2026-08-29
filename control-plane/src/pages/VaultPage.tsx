import { useEffect, useMemo, useState } from "react";
import { api, type Agent } from "../api/client";
import { teamsApi, type Team } from "../api/teams";
import {
  BODY_CAP,
  boundNeighborhood,
  capRows,
  classifyDoc,
  filterVaultDocs,
  formatMtime,
  GRAPH_NODE_CAP,
  isStaleHealth,
  LIST_CAP,
  plainVaultBody,
  shortHash,
  uniqueField,
  vaultApi,
  type VaultDoc,
  type VaultGraph,
  type VaultHealth,
  type VaultLink,
  type VaultSearchHit,
  type VaultSyncResult,
} from "../api/vault";
import { useI18n, type MsgKey } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { SectionHeader } from "../ui/SectionHeader";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

function typeLabelKey(type: string): MsgKey | null {
  if (type === "note") return "vault.type.note";
  if (type === "markdown") return "vault.type.markdown";
  if (type === "text") return "vault.type.text";
  if (type === "agent") return "vault.type.agent";
  if (type === "team") return "vault.type.team";
  if (type === "policy") return "vault.type.policy";
  return null;
}

export function VaultPage() {
  const { t } = useI18n();
  const [docs, setDocs] = useState<VaultDoc[]>([]);
  const [agents, setAgents] = useState<Agent[]>([]);
  const [teams, setTeams] = useState<Team[]>([]);
  const [selected, setSelected] = useState<VaultDoc | null>(null);
  const [inbound, setInbound] = useState<VaultLink[]>([]);
  const [outbound, setOutbound] = useState<VaultLink[]>([]);
  const [hits, setHits] = useState<VaultSearchHit[] | null>(null);
  const [sync, setSync] = useState<VaultSyncResult | null>(null);
  const [health, setHealth] = useState<VaultHealth | null>(null);
  const [graph, setGraph] = useState<VaultGraph | null>(null);
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(true);
  const [detailLoading, setDetailLoading] = useState(false);
  const [syncing, setSyncing] = useState(false);
  const [q, setQ] = useState("");
  const [typeFilter, setTypeFilter] = useState("");
  const [agentFilter, setAgentFilter] = useState("");
  const [teamFilter, setTeamFilter] = useState("");
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");

  const visible = useMemo(
    () => filterVaultDocs(docs, { query: q, type: typeFilter, agent: agentFilter, team: teamFilter }),
    [docs, q, typeFilter, agentFilter, teamFilter],
  );
  const listed = useMemo(() => capRows(visible, LIST_CAP), [visible]);
  const filterEmpty = !loading && docs.length > 0 && visible.length === 0;
  const stale = isStaleHealth(health);
  const typeOptions = useMemo(() => uniqueField(docs, "type"), [docs]);
  const agentOptions = useMemo(() => {
    const fromDocs = uniqueField(docs, "agent");
    const seen = new Set(fromDocs.map((x) => x.toLowerCase()));
    const extra: string[] = [];
    for (const a of agents) {
      const id = (a.agent_key || a.id || "").trim();
      if (!id || seen.has(id.toLowerCase())) continue;
      seen.add(id.toLowerCase());
      extra.push(id);
    }
    return [...fromDocs, ...extra];
  }, [docs, agents]);
  const teamOptions = useMemo(() => {
    const fromDocs = uniqueField(docs, "team");
    const seen = new Set(fromDocs.map((x) => x.toLowerCase()));
    const extra: string[] = [];
    for (const tm of teams) {
      const name = (tm.name || tm.id || "").trim();
      if (!name || seen.has(name.toLowerCase())) continue;
      seen.add(name.toLowerCase());
      extra.push(name);
    }
    return [...fromDocs, ...extra];
  }, [docs, teams]);
  const neighborhood = useMemo(
    () => boundNeighborhood(selected, inbound, outbound, docs, GRAPH_NODE_CAP),
    [selected, inbound, outbound, docs],
  );
  const graphView = graph && graph.nodes.length > 0 ? graph : neighborhood;
  const selectedClass = selected ? classifyDoc(selected) : null;
  const bodyText = selected ? plainVaultBody(selected.body) : "";
  const bodyTruncated = Boolean(selected?.body && selected.body.replace(/\u0000/g, "").length > BODY_CAP);

  async function loadHealth() {
    try {
      setHealth(await vaultApi.health());
    } catch {
      setHealth(null);
    }
  }

  async function loadGraph() {
    try {
      setGraph(await vaultApi.graph(GRAPH_NODE_CAP));
    } catch {
      setGraph(null);
    }
  }

  async function load() {
    setLoading(true);
    try {
      const [j, a, tm] = await Promise.all([
        vaultApi.list(),
        api.listAgents().catch(() => ({ agents: [] as Agent[] })),
        teamsApi.list().catch(() => ({ teams: [] as Team[] })),
      ]);
      setDocs(j.docs ?? []);
      setAgents(a.agents ?? []);
      setTeams(tm.teams ?? []);
      setErr("");
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setLoading(false);
    }
    void loadHealth();
    void loadGraph();
  }

  async function openDoc(id: string) {
    setDetailLoading(true);
    try {
      const [d, links] = await Promise.all([vaultApi.get(id), vaultApi.links(id)]);
      setSelected(d);
      setInbound(links.inbound ?? []);
      setOutbound(links.outbound ?? []);
      setTitle(d.title);
      setBody(d.body ?? "");
      setErr("");
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setDetailLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  async function search() {
    const query = q.trim();
    if (!query) {
      setHits(null);
      return;
    }
    try {
      setHits(await vaultApi.search(query));
      setErr("");
    } catch (e) {
      setErr(formatPublicError(e));
    }
  }

  async function putDoc() {
    if (!title.trim()) {
      setErr(t("vault.needTitle"));
      return;
    }
    try {
      const d = await vaultApi.put({ title: title.trim(), body });
      setSelected(d);
      setErr("");
      await load();
      await openDoc(d.id);
    } catch (e) {
      setErr(formatPublicError(e));
    }
  }

  async function runSync() {
    setSyncing(true);
    try {
      setSync(await vaultApi.sync());
      await load();
      if (selected?.id) await openDoc(selected.id);
      setErr("");
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setSyncing(false);
    }
  }

  function typeLabel(type: string): string {
    const key = typeLabelKey(type);
    return key ? t(key) : type || "—";
  }

  function agentLabel(id: string): string {
    const a = agents.find((x) => x.id === id || x.agent_key === id);
    if (!a) return id;
    return (a.display_name || "").trim() || a.agent_key || id;
  }

  function teamLabel(id: string): string {
    const tm = teams.find((x) => x.id === id || x.name === id);
    if (!tm) return id;
    return (tm.name || "").trim() || tm.id;
  }

  const shownHits = useMemo(() => {
    if (!hits) return null;
    return hits.filter((h) => {
      const doc = docs.find((d) => d.id === h.id) || { id: h.id, title: h.title, path: h.path };
      return filterVaultDocs([doc], { type: typeFilter, agent: agentFilter, team: teamFilter }).length > 0;
    });
  }, [hits, docs, typeFilter, agentFilter, teamFilter]);

  return (
    <div style={{ padding: "14px 22px 40px", display: "flex", flexDirection: "column", gap: 14 }}>
      <SectionHeader
        icon="doc"
        title={t("vault.title")}
        description={t("vault.desc")}
        actions={
          <>
            <Button icon="refresh" iconGesture onClick={() => void load()}>
              {t("common.refresh")}
            </Button>
            <Button icon="refresh" onClick={() => void runSync()} disabled={syncing}>
              {t("vault.sync")}
            </Button>
            <Button variant="primary" icon="plus" onClick={() => void putDoc()}>
              {t("vault.put")}
            </Button>
          </>
        }
      />
      {err ? <StatusLine kind="error">{err}</StatusLine> : null}
      {stale ? (
        <p role="status" style={{ margin: 0, fontSize: 12.5, color: "var(--orange)" }}>
          <Badge tone="warning">{t("vault.stale")}</Badge>{" "}
          {t("vault.staleDetail", {
            missing: health?.missing_on_disk ?? 0,
            unindexed: health?.unindexed ?? 0,
            mismatch: health?.hash_mismatch ?? 0,
          })}
        </p>
      ) : health ? (
        <p role="status" style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>
          {t("vault.healthOk")}
        </p>
      ) : null}
      {sync ? (
        <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-2)" }}>
          {t("vault.syncResult", { upserted: sync.upserted, skipped: sync.skipped, deleted: sync.deleted })}
        </p>
      ) : null}
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "center" }}>
        <input
          className="z-field"
          style={{ flex: 1, minWidth: 160 }}
          placeholder={t("vault.search")}
          value={q}
          onChange={(e) => setQ(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") void search();
          }}
          aria-label={t("vault.search")}
          autoComplete="off"
        />
        <Button icon="search" onClick={() => void search()}>
          {t("common.search")}
        </Button>
        <select
          className="z-field"
          aria-label={t("vault.filterType")}
          value={typeFilter}
          onChange={(e) => setTypeFilter(e.target.value)}
          style={{ minWidth: 140 }}
        >
          <option value="">{t("vault.filterTypeAll")}</option>
          {typeOptions.map((ty) => (
            <option key={ty} value={ty}>
              {typeLabel(ty)}
            </option>
          ))}
        </select>
        <select
          className="z-field"
          aria-label={t("vault.filterAgent")}
          value={agentFilter}
          onChange={(e) => setAgentFilter(e.target.value)}
          style={{ minWidth: 140 }}
        >
          <option value="">{t("vault.filterAgentAll")}</option>
          {agentOptions.map((id) => (
            <option key={id} value={id}>
              {agentLabel(id)}
            </option>
          ))}
        </select>
        <select
          className="z-field"
          aria-label={t("vault.filterTeam")}
          value={teamFilter}
          onChange={(e) => setTeamFilter(e.target.value)}
          style={{ minWidth: 140 }}
        >
          <option value="">{t("vault.filterTeamAll")}</option>
          {teamOptions.map((id) => (
            <option key={id} value={id}>
              {teamLabel(id)}
            </option>
          ))}
        </select>
      </div>
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
        <input className="z-field" placeholder={t("vault.titleField")} value={title} onChange={(e) => setTitle(e.target.value)} aria-label={t("vault.titleField")} />
      </div>
      <textarea
        className="z-field"
        placeholder={t("vault.body")}
        value={body}
        onChange={(e) => setBody(e.target.value)}
        rows={6}
        style={{ minHeight: 90, resize: "vertical" }}
        aria-label={t("vault.body")}
      />
      {shownHits ? (
        <Card>
          <CardHeader icon="search" title={t("vault.hits")} meta={String(shownHits.length)} />
          {shownHits.map((h) => (
            <button
              key={h.id}
              type="button"
              onClick={() => void openDoc(h.id)}
              style={{ display: "block", width: "100%", textAlign: "left", background: "transparent", border: "none", borderBottom: "1px solid var(--border-soft)", padding: "10px 16px", color: "var(--text)" }}
            >
              <div style={{ fontSize: 13, fontWeight: 600 }}>{h.title}</div>
              <div style={{ fontSize: 12, color: "var(--text-3)" }}>{h.snippet || h.path}</div>
            </button>
          ))}
          {shownHits.length === 0 ? <EmptyState>{t("vault.emptyHits")}</EmptyState> : null}
        </Card>
      ) : null}
      <div className="z-two-col">
        <Card>
          <CardHeader icon="doc" title={t("vault.list")} meta={t("vault.meta", { n: visible.length })} />
          <TableScroll>
          <div style={{ display: "flex", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)", gap: 8 }}>
            <span style={{ flex: 2 }}>{t("vault.col.title")}</span>
            <span style={{ flex: 1 }}>{t("vault.col.type")}</span>
            <span style={{ flex: 1 }}>{t("vault.col.agent")}</span>
            <span style={{ flex: 1 }}>{t("vault.col.team")}</span>
            <span style={{ flex: 2 }}>{t("vault.col.path")}</span>
          </div>
          {listed.rows.map((d) => {
            const c = classifyDoc(d);
            const on = selected?.id === d.id;
            return (
              <button
                key={d.id}
                type="button"
                onClick={() => void openDoc(d.id)}
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
                <span style={{ flex: 2, fontWeight: 600 }}>{d.title}</span>
                <span style={{ flex: 1 }}>{typeLabel(c.type)}</span>
                <span style={{ flex: 1, color: "var(--text-2)" }}>{c.agent ? agentLabel(c.agent) : "—"}</span>
                <span style={{ flex: 1, color: "var(--text-2)" }}>{c.team ? teamLabel(c.team) : "—"}</span>
                <span style={{ flex: 2, color: "var(--text-3)" }}>{d.path}</span>
              </button>
            );
          })}
          {loading ? <StatusLine kind="loading" /> : null}
          {!loading && docs.length === 0 ? <EmptyState>{t("vault.empty")}</EmptyState> : null}
          {filterEmpty ? <EmptyState>{t("vault.filterEmpty")}</EmptyState> : null}
          {listed.truncated ? (
            <p style={{ margin: 0, padding: "8px 16px", fontSize: 12, color: "var(--text-3)" }}>{t("vault.listTruncated", { n: LIST_CAP })}</p>
          ) : null}
          </TableScroll>
        </Card>
        <Card>
          <CardHeader icon="doc" title={t("vault.detail")} meta={selected?.title} />
          <div style={{ padding: "10px 16px", fontSize: 12.5 }}>
            {detailLoading ? <StatusLine kind="loading" /> : null}
            {!detailLoading && !selected ? <EmptyState style={{ padding: "24px 8px" }}>{t("vault.emptyDetail")}</EmptyState> : null}
            {selected && selectedClass ? (
              <>
                <dl style={{ display: "grid", gridTemplateColumns: "auto 1fr", gap: "4px 12px", margin: "0 0 12px" }}>
                  <dt style={{ color: "var(--text-3)" }}>{t("vault.metaId")}</dt>
                  <dd style={{ margin: 0 }}>{selected.id}</dd>
                  <dt style={{ color: "var(--text-3)" }}>{t("vault.metaPath")}</dt>
                  <dd style={{ margin: 0 }}>{selected.path}</dd>
                  <dt style={{ color: "var(--text-3)" }}>{t("vault.metaType")}</dt>
                  <dd style={{ margin: 0 }}>{typeLabel(selectedClass.type)}</dd>
                  <dt style={{ color: "var(--text-3)" }}>{t("vault.metaAgent")}</dt>
                  <dd style={{ margin: 0 }}>{selectedClass.agent ? agentLabel(selectedClass.agent) : "—"}</dd>
                  <dt style={{ color: "var(--text-3)" }}>{t("vault.metaTeam")}</dt>
                  <dd style={{ margin: 0 }}>{selectedClass.team ? teamLabel(selectedClass.team) : "—"}</dd>
                  <dt style={{ color: "var(--text-3)" }}>{t("vault.metaSha")}</dt>
                  <dd style={{ margin: 0 }}>{shortHash(selected.sha256)}</dd>
                  <dt style={{ color: "var(--text-3)" }}>{t("vault.metaMtime")}</dt>
                  <dd style={{ margin: 0 }}>{formatMtime(selected.mtime)}</dd>
                </dl>
                <div style={{ fontWeight: 600, marginBottom: 6 }}>{t("vault.outbound")}</div>
                {outbound.map((l, i) => (
                  <button
                    key={`o-${l.from_id}-${l.raw}-${i}`}
                    type="button"
                    disabled={!l.to_id}
                    onClick={() => {
                      if (l.to_id) void openDoc(l.to_id);
                    }}
                    style={{ display: "block", width: "100%", textAlign: "left", background: "transparent", border: "none", color: l.to_id ? "var(--accent)" : "var(--text-2)", padding: "3px 0", cursor: l.to_id ? "pointer" : "default" }}
                  >
                    [[{l.raw}]] {l.to_id ? `→ ${l.to_id}` : `(${t("vault.unresolved")})`}
                  </button>
                ))}
                {outbound.length === 0 ? <div style={{ color: "var(--text-4)", fontStyle: "italic" }}>{t("vault.emptyLinks")}</div> : null}
                <div style={{ fontWeight: 600, margin: "12px 0 6px" }}>{t("vault.inbound")}</div>
                {inbound.map((l, i) => (
                  <button
                    key={`i-${l.from_id}-${l.raw}-${i}`}
                    type="button"
                    onClick={() => void openDoc(l.from_id)}
                    style={{ display: "block", width: "100%", textAlign: "left", background: "transparent", border: "none", color: "var(--accent)", padding: "3px 0", cursor: "pointer" }}
                  >
                    {l.from_id} → [[{l.raw}]]
                  </button>
                ))}
                {inbound.length === 0 ? <div style={{ color: "var(--text-4)", fontStyle: "italic" }}>{t("vault.emptyLinks")}</div> : null}
                {bodyText ? (
                  <>
                    <pre style={{ marginTop: 12, whiteSpace: "pre-wrap", fontSize: 12, color: "var(--text-2)", fontFamily: "inherit" }}>{bodyText}</pre>
                    {bodyTruncated ? <p style={{ margin: "4px 0 0", fontSize: 11.5, color: "var(--text-3)" }}>{t("vault.bodyTruncated")}</p> : null}
                  </>
                ) : null}
              </>
            ) : null}
          </div>
        </Card>
      </div>
      <Card>
        <CardHeader icon="hook" title={t("vault.graph")} meta={t("vault.noCanvas")} />
        <div style={{ padding: "10px 16px", fontSize: 12.5 }}>
          {graphView.truncated ? (
            <p style={{ margin: "0 0 8px", color: "var(--orange)" }}>{t("vault.graphTruncated", { n: graphView.node_cap })}</p>
          ) : null}
          <div style={{ fontWeight: 600, marginBottom: 6 }}>{t("vault.nodes")}</div>
          {graphView.nodes.map((n) => (
            <button
              key={n.id}
              type="button"
              onClick={() => {
                if (!n.id.startsWith("raw:")) void openDoc(n.id);
              }}
              disabled={n.id.startsWith("raw:")}
              style={{
                display: "block",
                width: "100%",
                textAlign: "left",
                background: selected?.id === n.id ? "var(--accent-soft)" : "transparent",
                border: "none",
                borderBottom: "1px solid var(--border-soft)",
                padding: "8px 0",
                color: n.id.startsWith("raw:") ? "var(--text-2)" : "var(--text)",
                cursor: n.id.startsWith("raw:") ? "default" : "pointer",
              }}
            >
              <div style={{ fontWeight: 600 }}>{n.title}</div>
              <div style={{ fontSize: 11.5, color: "var(--text-3)" }}>{n.path || n.id}</div>
            </button>
          ))}
          {graphView.nodes.length === 0 ? <EmptyState style={{ padding: "16px 8px" }}>{t("vault.emptyGraph")}</EmptyState> : null}
          <div style={{ fontWeight: 600, margin: "12px 0 6px" }}>{t("vault.edges")}</div>
          {graphView.edges.map((e, i) => (
            <div key={`${e.from_id}-${e.raw}-${i}`} style={{ color: "var(--text-2)", padding: "3px 0" }}>
              {e.from_id} → [[{e.raw}]] {e.to_id ? `→ ${e.to_id}` : `(${t("vault.unresolved")})`}
            </div>
          ))}
          {graphView.edges.length === 0 ? <div style={{ color: "var(--text-4)", fontStyle: "italic" }}>{t("vault.emptyLinks")}</div> : null}
        </div>
      </Card>
    </div>
  );
}
