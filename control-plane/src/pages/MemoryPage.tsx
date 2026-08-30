import { useEffect, useMemo, useState } from "react";
import { resolveSettled } from "../api/channel-ops";
import { api, type Agent, type Session } from "../api/client";
import { confirmNamed } from "../api/confirm";
import {
  BODY_CAP,
  LIST_CAP,
  capRows,
  classifyMemoryList,
  filterMemories,
  hasBothLanes,
  isEmbeddingConfigured,
  listTargetName,
  memoryCreateBlocked,
  memoryFilteredEmpty,
  memoryFormBlocked,
  memoryLane,
  memoryMutationsBlocked,
  memorySnippet,
  normalizeKind,
  plainMemoryBody,
} from "../api/memory-ops";
import { memoryApi, type KgExpand, type MemoryHit, type MemoryIndex, type MemoryNote } from "../api/memory";
import { classifyPageState, formatStaleAt, listMetaCount } from "../api/page-state";
import { useI18n } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { PageChrome } from "../ui/PageChrome";
import { PageStatus } from "../ui/PageStatus";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

export function MemoryPage() {
  const { t, locale } = useI18n();
  const [sessions, setSessions] = useState<Session[]>([]);
  const [agents, setAgents] = useState<Agent[]>([]);
  const [notes, setNotes] = useState<MemoryNote[]>([]);
  const [selected, setSelected] = useState<MemoryNote | null>(null);
  const [hits, setHits] = useState<MemoryHit[] | null>(null);
  const [expand, setExpand] = useState<KgExpand | null>(null);
  const [expandId, setExpandId] = useState("");
  const [index, setIndex] = useState<MemoryIndex | null>(null);
  const [err, setErr] = useState<unknown>(null);
  const [agentsErr, setAgentsErr] = useState<unknown>(null);
  const [sessionsErr, setSessionsErr] = useState<unknown>(null);
  const [actionErr, setActionErr] = useState("");
  const [ok, setOk] = useState("");
  const [loading, setLoading] = useState(true);
  const [loaded, setLoaded] = useState(false);
  const [loadedAt, setLoadedAt] = useState<string | null>(null);
  const [sessionsLoaded, setSessionsLoaded] = useState(false);
  const [detailLoading, setDetailLoading] = useState(false);
  const [showForm, setShowForm] = useState(false);
  const [q, setQ] = useState("");
  const [body, setBody] = useState("");
  const [kind, setKind] = useState("episodic");
  const [sessionId, setSessionId] = useState("");
  const [agentFilter, setAgentFilter] = useState("");
  const [laneFilter, setLaneFilter] = useState("");

  const notesState = classifyMemoryList({ loading, loaded, error: err, itemCount: notes.length });
  const sessState = classifyPageState({
    loading: false,
    loaded: sessionsLoaded,
    error: sessionsErr,
    itemCount: sessions.length,
  });
  const blocked = memoryMutationsBlocked(notesState);
  const formBlocked = memoryFormBlocked(notesState, sessState);
  const createBlocked = memoryCreateBlocked(notesState, sessState, sessionId);
  const visible = useMemo(
    () => filterMemories(notesState.showItems ? notes : [], { query: q, agent: agentFilter, session: sessionId, lane: laneFilter }),
    [notes, q, agentFilter, sessionId, laneFilter, notesState.showItems],
  );
  const episodic = useMemo(() => visible.filter((n) => memoryLane(n.kind) === "episodic"), [visible]);
  const durable = useMemo(() => visible.filter((n) => memoryLane(n.kind) === "durable"), [visible]);
  const split = hasBothLanes(visible) && !laneFilter;
  const filterEmpty = memoryFilteredEmpty(notesState, notes.length, visible.length);
  const embedOn = isEmbeddingConfigured(index);
  const metaN = listMetaCount(notesState.kind, visible.length);
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
    const [memRes, sessRes, agRes] = await Promise.allSettled([
      memoryApi.list(),
      api.listSessions(),
      api.listAgents(),
    ]);
    const mem = resolveSettled(memRes);
    if (mem.ok) {
      setNotes(mem.value.memories ?? []);
      setLoaded(true);
      setLoadedAt(new Date().toISOString());
      setErr(null);
    } else {
      setErr(mem.error);
    }
    const sess = resolveSettled(sessRes);
    if (sess.ok) {
      setSessions(sess.value.sessions ?? []);
      setSessionsLoaded(true);
      setSessionsErr(null);
    } else {
      setSessionsErr(sess.error);
      setSessionsLoaded(false);
    }
    const ag = resolveSettled(agRes);
    if (ag.ok) {
      setAgents(ag.value.agents ?? []);
      setAgentsErr(null);
    } else {
      setAgentsErr(ag.error);
    }
    setLoading(false);
    void loadIndex();
  }

  async function openNote(id: string) {
    if (blocked) return;
    setDetailLoading(true);
    try {
      const n = await memoryApi.get(id);
      setSelected(n);
      setBody(n.body ?? "");
      setKind(normalizeKind(n.kind) === "episodic" ? "episodic" : "durable");
      setActionErr("");
    } catch (e) {
      setActionErr(formatPublicError(e));
    } finally {
      setDetailLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  async function postNote() {
    if (createBlocked) return;
    if (!body.trim()) {
      setActionErr(t("memory.needNote"));
      return;
    }
    try {
      const n = await memoryApi.create({ session_id: sessionId, body: body.trim(), kind });
      setBody("");
      setShowForm(false);
      setOk(t("memory.savedOk"));
      setActionErr("");
      await load();
      await openNote(n.id);
    } catch (e) {
      setActionErr(formatPublicError(e));
    }
  }

  async function saveNote() {
    if (blocked || !selected) return;
    if (!body.trim()) {
      setActionErr(t("memory.needNote"));
      return;
    }
    try {
      const n = await memoryApi.patch(selected.id, { body: body.trim(), kind });
      setSelected(n);
      setOk(t("memory.updatedOk"));
      setActionErr("");
      await load();
    } catch (e) {
      setActionErr(formatPublicError(e));
    }
  }

  async function deleteNote() {
    if (blocked || !selected) return;
    const named = listTargetName({ snippet: selected.snippet || memorySnippet(selected.body), id: selected.id, kind: selected.kind });
    if (!confirmNamed(t("memory.confirmDelete", { name: named }), (m) => window.confirm(m))) return;
    try {
      await memoryApi.remove(selected.id);
      setSelected(null);
      setBody("");
      setOk(t("memory.deletedOk"));
      setActionErr("");
      await load();
    } catch (e) {
      setActionErr(formatPublicError(e));
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
      setActionErr("");
    } catch (e) {
      setActionErr(formatPublicError(e));
    }
  }

  async function expandHit(id: string, tier?: string) {
    if (!id || tier !== "l2") {
      setActionErr(t("memory.expandNeed"));
      return;
    }
    try {
      setExpand(await memoryApi.expand(id));
      setExpandId(id);
      setActionErr("");
    } catch (e) {
      setActionErr(formatPublicError(e));
    }
  }

  const shownHits = useMemo(() => {
    if (!hits || blocked) return null;
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
  }, [hits, notes, sessions, agentFilter, sessionId, laneFilter, blocked]);

  function renderList(title: string, rows: MemoryNote[], empty: string) {
    const listed = capRows(rows, LIST_CAP);
    return (
      <Card>
        <CardHeader icon="inbox" title={title} meta={metaN == null ? "—" : t("memory.meta", { n: rows.length })} />
        <TableScroll>
          <div style={{ display: "flex", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)", gap: 8 }}>
            <span style={{ flex: 1 }}>{t("memory.col.kind")}</span>
            <span style={{ flex: 1 }}>{t("memory.col.agent")}</span>
            <span style={{ flex: 1 }}>{t("memory.col.session")}</span>
            <span style={{ flex: 3 }}>{t("memory.col.body")}</span>
          </div>
          {notesState.showItems
            ? listed.rows.map((n) => {
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
              })
            : null}
          {notesState.showEmpty ? <EmptyState data-page-state="empty">{empty}</EmptyState> : null}
          {listed.truncated ? (
            <p style={{ margin: 0, padding: "8px 16px", fontSize: 12, color: "var(--text-3)" }}>{t("memory.listTruncated", { n: LIST_CAP })}</p>
          ) : null}
        </TableScroll>
      </Card>
    );
  }

  const bodyText = selected ? plainMemoryBody(selected.body) : "";
  const bodyTruncated = Boolean(selected?.body && selected.body.replace(/\u0000/g, "").length > BODY_CAP);
  const showLists = notesState.showItems || notesState.showEmpty || filterEmpty;

  return (
    <PageChrome
      icon="inbox"
      title={t("memory.title")}
      description={t("memory.desc")}
      primary={
        <Button
          variant="primary"
          icon="plus"
          disabled={formBlocked || sessState.kind === "empty"}
          onClick={() => {
            if (formBlocked || sessState.kind === "empty") return;
            setShowForm(true);
            setActionErr("");
            setOk("");
          }}
        >
          {t("memory.post")}
        </Button>
      }
      refresh={
        <Button icon="refresh" iconGesture onClick={() => void load()} disabled={loading}>
          {t("common.refresh")}
        </Button>
      }
      filters={
        <>
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
        </>
      }
    >
      <p role="status" style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>
        {index?.fts ? t("memory.index.fts") : t("memory.index.substring")}{" "}
        {embedOn ? <Badge tone="neutral">{t("memory.index.embedOn")}</Badge> : <Badge tone="warning">{t("memory.index.embedOff")}</Badge>}{" "}
        {!embedOn ? t("memory.index.embedGuide") : null}
      </p>
      <PageStatus kind={notesState.kind} errorText={err ? formatPublicError(err) : ""} staleAt={formatStaleAt(loadedAt, locale)} onReload={() => void load()} />
      {agentsErr ? (
        <div data-page-state="dependency">
          <StatusLine kind="error">
            {t("memory.agentsUnavailable")} · {formatPublicError(agentsErr)}
          </StatusLine>
        </div>
      ) : null}
      {sessionsErr ? (
        <div data-page-state="dependency">
          <StatusLine kind="error">
            {t("memory.sessionsUnavailable")} · {formatPublicError(sessionsErr)}
          </StatusLine>
        </div>
      ) : null}
      {sessState.showEmpty && !sessionsErr ? <EmptyState data-page-state="dependency">{t("memory.noSessions")}</EmptyState> : null}
      {actionErr ? <StatusLine kind="error">{actionErr}</StatusLine> : null}
      {ok && !actionErr ? (
        <p role="status" style={{ margin: 0, fontSize: 12.5, color: "var(--green)" }}>
          {ok}
        </p>
      ) : null}
      {!formBlocked && showForm ? (
        <Card>
          <CardHeader icon="plus" title={t("memory.formTitle")} />
          <div style={{ padding: "0 16px 16px", display: "flex", flexDirection: "column", gap: 10 }}>
            <select className="z-field" value={kind} onChange={(e) => setKind(e.target.value)} aria-label={t("memory.kind")}>
              <option value="episodic">{t("memory.kindEpisodic")}</option>
              <option value="durable">{t("memory.kindDurable")}</option>
            </select>
            <select className="z-field" value={sessionId} onChange={(e) => setSessionId(e.target.value)} aria-label={t("memory.session")}>
              <option value="">{t("memory.pickSession")}</option>
              {sessionOptions.map((s) => (
                <option key={s.id} value={s.id}>
                  {sessionLabel(s.id)}
                </option>
              ))}
            </select>
            <textarea
              className="z-field"
              placeholder={t("memory.note")}
              value={body}
              onChange={(e) => setBody(e.target.value)}
              rows={3}
              style={{ minHeight: 64, resize: "vertical" }}
              aria-label={t("memory.note")}
            />
            <div style={{ display: "flex", gap: 8 }}>
              <Button variant="primary" disabled={createBlocked} onClick={() => void postNote()}>
                {t("memory.post")}
              </Button>
              <Button variant="quiet" onClick={() => setShowForm(false)}>
                {t("common.cancel")}
              </Button>
            </div>
          </div>
        </Card>
      ) : null}
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
      {showLists ? (
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
            {filterEmpty ? <EmptyState data-page-state="filtered_empty">{t("memory.filterEmpty")}</EmptyState> : null}
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
                    <Button variant="primary" disabled={blocked} onClick={() => void saveNote()}>
                      {t("memory.save")}
                    </Button>
                    <Button disabled={blocked} onClick={() => void deleteNote()}>
                      {t("common.delete")}
                    </Button>
                  </div>
                </>
              ) : null}
            </div>
          </Card>
        </div>
      ) : null}
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
    </PageChrome>
  );
}
