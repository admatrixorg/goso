import { useEffect, useMemo, useState } from "react";
import { api, type Agent, type Session } from "../api/client";
import {
  BODY_CAP,
  LIST_CAP,
  capRows,
  filterMemories,
  hasBothLanes,
  isEmbeddingConfigured,
  listTargetName,
  memoryLane,
  memorySnippet,
  normalizeKind,
  plainMemoryBody,
} from "../api/memory-ops";
import { memoryApi, type KgExpand, type MemoryHit, type MemoryIndex, type MemoryNote } from "../api/memory";
import { useI18n } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { SectionHeader } from "../ui/SectionHeader";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

export function MemoryPage() {
  const { t } = useI18n();
  const [sessions, setSessions] = useState<Session[]>([]);
  const [agents, setAgents] = useState<Agent[]>([]);
  const [notes, setNotes] = useState<MemoryNote[]>([]);
  const [selected, setSelected] = useState<MemoryNote | null>(null);
  const [hits, setHits] = useState<MemoryHit[] | null>(null);
  const [expand, setExpand] = useState<KgExpand | null>(null);
  const [expandId, setExpandId] = useState("");
  const [index, setIndex] = useState<MemoryIndex | null>(null);
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(true);
  const [detailLoading, setDetailLoading] = useState(false);
  const [q, setQ] = useState("");
  const [body, setBody] = useState("");
  const [kind, setKind] = useState("episodic");
  const [sessionId, setSessionId] = useState("");
  const [agentFilter, setAgentFilter] = useState("");
  const [laneFilter, setLaneFilter] = useState("");

  const visible = useMemo(
    () => filterMemories(notes, { query: q, agent: agentFilter, session: sessionId, lane: laneFilter }),
    [notes, q, agentFilter, sessionId, laneFilter],
  );
  const episodic = useMemo(() => visible.filter((n) => memoryLane(n.kind) === "episodic"), [visible]);
  const durable = useMemo(() => visible.filter((n) => memoryLane(n.kind) === "durable"), [visible]);
  const split = hasBothLanes(visible) && !laneFilter;
  const filterEmpty = !loading && notes.length > 0 && visible.length === 0;
  const embedOn = isEmbeddingConfigured(index);
  const sessionOptions = useMemo(() => {
    if (!agentFilter) return sessions;
    return sessions.filter((s) => s.agent_id === agentFilter);
  }, [sessions, agentFilter]);

  function agentLabel(id: string): string {
    const a = agents.find((x) => x.id === id || x.agent_key === id);
    if (!a) return id;
    return (a.display_name || "").trim() || a.agent_key || id;
  }

  function sessionLabel(id: string): string {
    const s = sessions.find((x) => x.id === id);
    if (!s) return id;
    const name = (s.label || "").trim();
    return name || id;
  }

  function laneLabel(k: string): string {
    return memoryLane(k) === "durable" ? t("memory.lane.durable") : t("memory.lane.episodic");
  }

  async function loadIndex() {
    try {
      setIndex(await memoryApi.index());
    } catch {
      setIndex({ search: "substring", fts: false, embedding: "not_configured", embedding_configured: false });
    }
  }

  async function load() {
    setLoading(true);
    try {
      const [sess, ag, mem] = await Promise.all([
        api.listSessions(),
        api.listAgents().catch(() => ({ agents: [] as Agent[] })),
        memoryApi.list({
          session_id: sessionId || undefined,
          agent_id: agentFilter || undefined,
        }),
      ]);
      setSessions(sess.sessions ?? []);
      setAgents(ag.agents ?? []);
      setNotes(mem.memories ?? []);
      setErr("");
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setLoading(false);
    }
    void loadIndex();
  }

  async function openNote(id: string) {
    setDetailLoading(true);
    try {
      const n = await memoryApi.get(id);
      setSelected(n);
      setBody(n.body ?? "");
      setKind(normalizeKind(n.kind) === "episodic" ? "episodic" : "durable");
      setErr("");
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setDetailLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, [sessionId, agentFilter]);

  async function postNote() {
    if (!sessionId) {
      setErr(t("memory.needSession"));
      return;
    }
    if (!body.trim()) {
      setErr(t("memory.needNote"));
      return;
    }
    try {
      const n = await memoryApi.create({ session_id: sessionId, body: body.trim(), kind });
      setBody("");
      setErr("");
      await load();
      await openNote(n.id);
    } catch (e) {
      setErr(formatPublicError(e));
    }
  }

  async function saveNote() {
    if (!selected) {
      setErr(t("memory.needPick"));
      return;
    }
    if (!body.trim()) {
      setErr(t("memory.needNote"));
      return;
    }
    try {
      const n = await memoryApi.patch(selected.id, { body: body.trim(), kind });
      setSelected(n);
      setErr("");
      await load();
    } catch (e) {
      setErr(formatPublicError(e));
    }
  }

  async function deleteNote() {
    if (!selected) {
      setErr(t("memory.needPick"));
      return;
    }
    const named = listTargetName({ snippet: selected.snippet || memorySnippet(selected.body), id: selected.id, kind: selected.kind });
    if (!window.confirm(t("memory.confirmDelete", { name: named }))) return;
    try {
      await memoryApi.remove(selected.id);
      setSelected(null);
      setBody("");
      setErr("");
      await load();
    } catch (e) {
      setErr(formatPublicError(e));
    }
  }

  async function search() {
    const query = q.trim();
    if (!query) {
      setHits(null);
      return;
    }
    try {
      setHits(await memoryApi.searchProgressive(query));
      setExpand(null);
      setExpandId("");
      setErr("");
    } catch (e) {
      setErr(formatPublicError(e));
    }
  }

  async function expandHit(id: string, tier?: string) {
    if (!id || tier !== "l2") {
      setErr(t("memory.expandNeed"));
      return;
    }
    try {
      setExpand(await memoryApi.expand(id));
      setExpandId(id);
      setErr("");
    } catch (e) {
      setErr(formatPublicError(e));
    }
  }

  const shownHits = useMemo(() => {
    if (!hits) return null;
    return hits.filter((h) => {
      if (agentFilter) {
        const note = notes.find((n) => n.id === h.id);
        const sess = sessions.find((s) => s.id === (h.session_id || note?.session_id));
        const aid = note?.agent_id || sess?.agent_id || "";
        if (aid !== agentFilter) return false;
      }
      if (sessionId && (h.session_id || "") !== sessionId) {
        const note = notes.find((n) => n.id === h.id);
        if ((note?.session_id || "") !== sessionId) return false;
      }
      if (laneFilter === "episodic" || laneFilter === "durable") {
        if (memoryLane(h.kind || h.tier) !== laneFilter && !(laneFilter === "durable" && h.tier === "l2")) return false;
        if (laneFilter === "episodic" && h.tier === "l2") return false;
      }
      return true;
    });
  }, [hits, notes, sessions, agentFilter, sessionId, laneFilter]);

  function renderList(title: string, rows: MemoryNote[], empty: string) {
    const listed = capRows(rows, LIST_CAP);
    return (
      <Card>
        <CardHeader icon="inbox" title={title} meta={t("memory.meta", { n: rows.length })} />
        <TableScroll>
          <div style={{ display: "flex", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)", gap: 8 }}>
            <span style={{ flex: 1 }}>{t("memory.col.kind")}</span>
            <span style={{ flex: 1 }}>{t("memory.col.agent")}</span>
            <span style={{ flex: 1 }}>{t("memory.col.session")}</span>
            <span style={{ flex: 3 }}>{t("memory.col.body")}</span>
          </div>
          {listed.rows.map((n) => {
            const on = selected?.id === n.id;
            return (
              <button
                key={n.id}
                type="button"
                onClick={() => void openNote(n.id)}
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
                <span style={{ flex: 1, fontWeight: 600 }}>{laneLabel(n.kind)}</span>
                <span style={{ flex: 1, color: "var(--text-2)" }}>{n.agent_id ? agentLabel(n.agent_id) : "—"}</span>
                <span style={{ flex: 1, color: "var(--text-3)" }}>{sessionLabel(n.session_id)}</span>
                <span style={{ flex: 3, color: "var(--text-2)" }}>{memorySnippet(n.snippet || n.body)}</span>
              </button>
            );
          })}
          {loading ? <StatusLine kind="loading" /> : null}
          {!loading && rows.length === 0 ? <EmptyState>{empty}</EmptyState> : null}
          {listed.truncated ? (
            <p style={{ margin: 0, padding: "8px 16px", fontSize: 12, color: "var(--text-3)" }}>{t("memory.listTruncated", { n: LIST_CAP })}</p>
          ) : null}
        </TableScroll>
      </Card>
    );
  }

  const bodyText = selected ? plainMemoryBody(selected.body) : "";
  const bodyTruncated = Boolean(selected?.body && selected.body.replace(/\u0000/g, "").length > BODY_CAP);

  return (
    <div style={{ padding: "14px 22px 40px", display: "flex", flexDirection: "column", gap: 14 }}>
      <SectionHeader
        icon="inbox"
        title={t("memory.title")}
        description={t("memory.desc")}
        actions={
          <>
            <Button icon="refresh" iconGesture onClick={() => void load()}>
              {t("common.refresh")}
            </Button>
            <Button variant="primary" icon="plus" onClick={() => void postNote()}>
              {t("memory.post")}
            </Button>
          </>
        }
      />
      {err ? <StatusLine kind="error">{err}</StatusLine> : null}
      <p role="status" style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>
        {index?.fts ? t("memory.index.fts") : t("memory.index.substring")}{" "}
        {!embedOn ? (
          <>
            <Badge tone="warning">{t("memory.index.embedOff")}</Badge> {t("memory.index.embedGuide")}
          </>
        ) : null}
      </p>
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "center" }}>
        <input
          className="z-field"
          style={{ flex: 1, minWidth: 160 }}
          placeholder={t("memory.search")}
          value={q}
          onChange={(e) => setQ(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") void search();
          }}
          aria-label={t("memory.search")}
          autoComplete="off"
        />
        <Button icon="search" onClick={() => void search()}>
          {t("common.search")}
        </Button>
        <select className="z-field" value={agentFilter} onChange={(e) => setAgentFilter(e.target.value)} aria-label={t("memory.agent")} style={{ minWidth: 140 }}>
          <option value="">{t("memory.filterAgentAll")}</option>
          {agents.map((a) => (
            <option key={a.id} value={a.id}>
              {agentLabel(a.id)}
            </option>
          ))}
        </select>
        <select className="z-field" value={sessionId} onChange={(e) => setSessionId(e.target.value)} aria-label={t("memory.session")} style={{ minWidth: 140 }}>
          <option value="">{t("memory.filterSessionAll")}</option>
          {sessionOptions.map((s) => (
            <option key={s.id} value={s.id}>
              {sessionLabel(s.id)}
            </option>
          ))}
        </select>
        <select className="z-field" value={laneFilter} onChange={(e) => setLaneFilter(e.target.value)} aria-label={t("memory.filterLane")} style={{ minWidth: 140 }}>
          <option value="">{t("memory.filterLaneAll")}</option>
          <option value="episodic">{t("memory.lane.episodic")}</option>
          <option value="durable">{t("memory.lane.durable")}</option>
        </select>
      </div>
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
        <select className="z-field" value={kind} onChange={(e) => setKind(e.target.value)} aria-label={t("memory.kind")} style={{ minWidth: 140 }}>
          <option value="episodic">{t("memory.kindEpisodic")}</option>
          <option value="durable">{t("memory.kindDurable")}</option>
        </select>
        <textarea
          className="z-field"
          placeholder={t("memory.note")}
          value={body}
          onChange={(e) => setBody(e.target.value)}
          rows={3}
          style={{ flex: 1, minWidth: 180, minHeight: 64, resize: "vertical" }}
          aria-label={t("memory.note")}
        />
      </div>
      {shownHits ? (
        <Card>
          <CardHeader icon="search" title={t("memory.hits")} meta={String(shownHits.length)} />
          <TableScroll>
            <div style={{ display: "flex", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)" }}>
              <span style={{ flex: 1 }}>{t("memory.col.tier")}</span>
              <span style={{ flex: 1 }}>{t("memory.col.kind")}</span>
              <span style={{ flex: 2 }}>{t("memory.col.session")}</span>
              <span style={{ flex: 3 }}>{t("memory.col.body")}</span>
              <span style={{ width: 88 }} />
            </div>
            {shownHits.map((h) => (
              <div key={h.id} style={{ display: "flex", padding: "11px 16px", fontSize: 12.5, borderBottom: "1px solid var(--border-soft)", alignItems: "center" }}>
                <span style={{ flex: 1 }}>{h.tier || "l1"}</span>
                <span style={{ flex: 1 }}>{h.kind || h.name || ""}</span>
                <span style={{ flex: 2, color: "var(--text-3)" }}>{h.session_id ? sessionLabel(h.session_id) : h.name || ""}</span>
                <span style={{ flex: 3, color: "var(--text-2)" }}>{memorySnippet(h.snippet)}</span>
                <span style={{ width: 88 }}>
                  {h.tier === "l2" ? (
                    <Button variant="quiet" onClick={() => void expandHit(h.id, h.tier)}>
                      {t("memory.expand")}
                    </Button>
                  ) : h.kind !== "message" ? (
                    <Button variant="quiet" onClick={() => void openNote(h.id)}>
                      {t("memory.detail")}
                    </Button>
                  ) : null}
                </span>
              </div>
            ))}
            {shownHits.length === 0 ? <EmptyState>{t("memory.emptyHits")}</EmptyState> : null}
          </TableScroll>
        </Card>
      ) : null}
      <div className="z-two-col">
        <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
          {split ? (
            <>
              {renderList(t("memory.listEpisodic"), episodic, t("memory.emptyEpisodic"))}
              {renderList(t("memory.listDurable"), durable, t("memory.emptyDurable"))}
            </>
          ) : laneFilter === "durable" ? (
            renderList(t("memory.listDurable"), durable, t("memory.emptyDurable"))
          ) : laneFilter === "episodic" ? (
            renderList(t("memory.listEpisodic"), episodic, t("memory.emptyEpisodic"))
          ) : (
            renderList(t("memory.list"), visible, t("memory.empty"))
          )}
        </div>
        <Card>
          <CardHeader icon="inbox" title={t("memory.detail")} meta={selected ? memorySnippet(selected.snippet || selected.body, 40) : undefined} />
          <div style={{ padding: "10px 16px", fontSize: 12.5 }}>
            {detailLoading ? <StatusLine kind="loading" /> : null}
            {!detailLoading && !selected ? <EmptyState style={{ padding: "24px 8px" }}>{t("memory.emptyDetail")}</EmptyState> : null}
            {selected && !detailLoading ? (
              <>
                <dl style={{ display: "grid", gridTemplateColumns: "auto 1fr", gap: "4px 12px", margin: "0 0 12px" }}>
                  <dt style={{ color: "var(--text-3)" }}>{t("memory.metaId")}</dt>
                  <dd style={{ margin: 0 }}>{selected.id}</dd>
                  <dt style={{ color: "var(--text-3)" }}>{t("memory.metaScope")}</dt>
                  <dd style={{ margin: 0 }}>{laneLabel(selected.kind)}</dd>
                  <dt style={{ color: "var(--text-3)" }}>{t("memory.metaKind")}</dt>
                  <dd style={{ margin: 0 }}>{selected.kind}</dd>
                  <dt style={{ color: "var(--text-3)" }}>{t("memory.metaAgent")}</dt>
                  <dd style={{ margin: 0 }}>{selected.agent_id ? agentLabel(selected.agent_id) : "—"}</dd>
                  <dt style={{ color: "var(--text-3)" }}>{t("memory.metaSession")}</dt>
                  <dd style={{ margin: 0 }}>{sessionLabel(selected.session_id)}</dd>
                  <dt style={{ color: "var(--text-3)" }}>{t("memory.metaCreated")}</dt>
                  <dd style={{ margin: 0 }}>{selected.created_at || "—"}</dd>
                </dl>
                {bodyText ? (
                  <>
                    <pre style={{ margin: "0 0 8px", whiteSpace: "pre-wrap", fontSize: 12, color: "var(--text-2)", fontFamily: "inherit" }}>{bodyText}</pre>
                    {bodyTruncated ? <p style={{ margin: "0 0 8px", fontSize: 11.5, color: "var(--text-3)" }}>{t("memory.bodyTruncated")}</p> : null}
                  </>
                ) : null}
                <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
                  <Button variant="primary" onClick={() => void saveNote()}>
                    {t("memory.save")}
                  </Button>
                  <Button onClick={() => void deleteNote()}>{t("common.delete")}</Button>
                </div>
              </>
            ) : null}
          </div>
        </Card>
      </div>
      {filterEmpty ? <EmptyState>{t("memory.filterEmpty")}</EmptyState> : null}
      <Card>
        <CardHeader icon="layers" title={t("memory.expandTitle")} meta={expandId || t("memory.expandEmpty")} />
        {expand?.entity ? (
          <div style={{ padding: "12px 16px", display: "flex", flexDirection: "column", gap: 10 }}>
            <div style={{ fontSize: 13, fontWeight: 600 }}>{expand.entity.name}</div>
            <div style={{ fontSize: 12, color: "var(--text-3)" }}>{expand.entity.kind}</div>
            {expand.entity.body ? (
              <pre style={{ margin: 0, whiteSpace: "pre-wrap", fontSize: 12.5, color: "var(--text-2)", fontFamily: "inherit" }}>{plainMemoryBody(expand.entity.body)}</pre>
            ) : null}
            <div style={{ fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)" }}>{t("memory.relations")}</div>
            {(expand.relations ?? []).length === 0 ? (
              <EmptyState>{t("memory.rel.empty")}</EmptyState>
            ) : (
              <TableScroll>
                <div style={{ display: "flex", padding: "8px 0", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)" }}>
                  <span style={{ flex: 2 }}>{t("memory.rel.from")}</span>
                  <span style={{ flex: 1 }}>{t("memory.rel.kind")}</span>
                  <span style={{ flex: 2 }}>{t("memory.rel.to")}</span>
                </div>
                {(expand.relations ?? []).map((rel) => (
                  <div key={rel.id} style={{ display: "flex", padding: "10px 0", fontSize: 12.5, borderBottom: "1px solid var(--border-soft)" }}>
                    <span style={{ flex: 2 }}>{rel.from_name || rel.from_id}</span>
                    <span style={{ flex: 1 }}>{rel.rel}</span>
                    <span style={{ flex: 2 }}>{rel.to_name || rel.to_id}</span>
                  </div>
                ))}
              </TableScroll>
            )}
          </div>
        ) : (
          <EmptyState>{t("memory.expandEmpty")}</EmptyState>
        )}
      </Card>
    </div>
  );
}
