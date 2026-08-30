import { forwardRef, useEffect, useImperativeHandle, useMemo, useRef, useState } from "react";
import { api, PROMPT_MODES, type Agent, type Session } from "../api/client";
import { confirmNamed } from "../api/confirm";
import {
  classifyPageState,
  clampPageOffset,
  formatStaleAt,
  inventoryBlocksMutation,
  isFilteredEmpty,
  listMetaCount,
  pageSlice,
} from "../api/page-state";
import {
  SESSION_PAGE_SIZE,
  SESSION_PAGE_SIZES,
  agentLabel,
  filterSessions,
  normalizePromptMode,
  sessionActivityAt,
  sessionDisplayName,
} from "../api/sessions";
import { useI18n, type MsgKey } from "../i18n";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { PageChrome } from "../ui/PageChrome";
import { PageStatus } from "../ui/PageStatus";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

export type SessionsPageHandle = {
  focusCreate: () => void;
};

function promptModeKey(mode: string): MsgKey {
  if (mode === "task") return "promptMode.task";
  if (mode === "minimal") return "promptMode.minimal";
  if (mode === "none") return "promptMode.none";
  return "promptMode.full";
}

export const SessionsPage = forwardRef<
  SessionsPageHandle,
  {
    onPick: (id: string, label?: string) => void;
    compact?: boolean;
    selectedId?: string;
    onDeleted?: (id: string) => void;
  }
>(function SessionsPage({ onPick, compact, selectedId, onDeleted }, ref) {
  const { t, locale } = useI18n();
  const [sessions, setSessions] = useState<Session[]>([]);
  const [agents, setAgents] = useState<Agent[]>([]);
  const [err, setErr] = useState<unknown>(null);
  const [formErr, setFormErr] = useState("");
  const [loaded, setLoaded] = useState(false);
  const [loadedAt, setLoadedAt] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [deletingId, setDeletingId] = useState("");
  const [agentId, setAgentId] = useState("");
  const [label, setLabel] = useState("");
  const [query, setQuery] = useState("");
  const [filterAgent, setFilterAgent] = useState("");
  const [createOpen, setCreateOpen] = useState(false);
  const [offset, setOffset] = useState(0);
  const [pageSize, setPageSize] = useState(SESSION_PAGE_SIZE);
  const createBoxRef = useRef<HTMLDivElement>(null);
  const agentSelectRef = useRef<HTMLSelectElement>(null);
  const busy = creating || Boolean(deletingId);
  const state = classifyPageState({
    loading,
    loaded,
    error: err,
    itemCount: sessions.length,
    keepStale: loaded && sessions.length > 0,
  });
  const createBlocked = inventoryBlocksMutation(state.kind);
  const formVisible = !createBlocked && createOpen;

  useImperativeHandle(ref, () => ({
    focusCreate() {
      if (createBlocked) return;
      setCreateOpen(true);
      createBoxRef.current?.scrollIntoView({ behavior: "smooth", block: "nearest" });
      agentSelectRef.current?.focus();
    },
  }));

  async function load() {
    setLoading(true);
    const [sessRes, agRes] = await Promise.allSettled([api.listSessions(), api.listAgents()]);
    if (sessRes.status === "fulfilled") {
      setSessions(sessRes.value.sessions ?? []);
      setLoaded(true);
      setLoadedAt(new Date().toISOString());
      setErr(null);
    } else {
      setErr(sessRes.reason);
    }
    if (agRes.status === "fulfilled") {
      setAgents(agRes.value.agents ?? []);
    }
    setFormErr("");
    setLoading(false);
  }

  useEffect(() => {
    void load();
  }, []);

  const visible = useMemo(
    () => filterSessions(sessions, { query, agentId: filterAgent }),
    [sessions, query, filterAgent],
  );
  const safeOffset = clampPageOffset(visible.length, offset, pageSize);
  const page = useMemo(() => pageSlice(visible, safeOffset, pageSize), [visible, safeOffset, pageSize]);
  const filteredEmpty = isFilteredEmpty(state, sessions.length, visible.length);
  const noAgents = !createBlocked && state.kind !== "loading" && agents.length === 0;
  const errText = err ? formatPublicError(err) : "";
  const metaN = listMetaCount(state.kind, visible.length);
  const pages = Math.max(1, Math.ceil(visible.length / pageSize) || 1);
  const pageNo = visible.length === 0 ? 1 : Math.floor(safeOffset / pageSize) + 1;
  const last = Math.max(0, safeOffset + page.length);

  async function create() {
    if (busy || loading || createBlocked) return;
    setFormErr("");
    if (agents.length === 0) {
      setFormErr(t("sessions.noAgents"));
      return;
    }
    const picked = agentId.trim();
    if (!picked) {
      setFormErr(t("sessions.needAgent"));
      return;
    }
    const trimmedLabel = label.trim();
    setCreating(true);
    try {
      const created = await api.createSession(trimmedLabel ? { agent_id: picked, label: trimmedLabel } : { agent_id: picked });
      setLabel("");
      setCreateOpen(false);
      setSessions((prev) => [created, ...prev.filter((s) => s.id !== created.id)]);
      setLoaded(true);
      setLoadedAt(new Date().toISOString());
      setErr(null);
      setFormErr("");
      onPick(created.id, sessionDisplayName(created));
    } catch (e) {
      setFormErr(formatPublicError(e));
    } finally {
      setCreating(false);
    }
  }

  async function persistMode(id: string, next: string) {
    const mode = normalizePromptMode(next);
    try {
      const updated = await api.updateSession(id, { prompt_mode: mode });
      setSessions((prev) => prev.map((s) => (s.id === id ? { ...s, prompt_mode: updated.prompt_mode || mode } : s)));
      setFormErr("");
    } catch (e) {
      setFormErr(formatPublicError(e));
    }
  }

  async function remove(s: Session) {
    if (busy || createBlocked) return;
    const named = sessionDisplayName(s);
    if (!confirmNamed(t("sessions.confirmDelete", { name: named }), (m) => window.confirm(m))) return;
    setDeletingId(s.id);
    try {
      await api.deleteSession(s.id);
      setSessions((prev) => prev.filter((row) => row.id !== s.id));
      setFormErr("");
      onDeleted?.(s.id);
    } catch (e) {
      setFormErr(formatPublicError(e));
    } finally {
      setDeletingId("");
    }
  }

  function filterBar() {
    return (
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "center" }}>
        <input
          className="z-field"
          style={{ flex: 1, minWidth: compact ? 0 : 160 }}
          value={query}
          onChange={(e) => {
            setQuery(e.target.value);
            setOffset(0);
          }}
          placeholder={t("sessions.search")}
          aria-label={t("sessions.search")}
          autoComplete="off"
        />
        <select
          className="z-field"
          aria-label={t("sessions.filterAgent")}
          value={filterAgent}
          onChange={(e) => {
            setFilterAgent(e.target.value);
            setOffset(0);
          }}
          style={{ minWidth: compact ? 0 : 160, flex: compact ? 1 : undefined }}
        >
          <option value="">{t("sessions.filterAgentAll")}</option>
          {agents.map((a) => (
            <option key={a.id} value={a.id}>
              {a.display_name || a.agent_key}
            </option>
          ))}
        </select>
      </div>
    );
  }

  function createFields() {
    return (
      <div ref={createBoxRef} style={{ display: "flex", flexDirection: "column", gap: compact ? 8 : 10 }}>
        {noAgents ? (
          <EmptyState style={{ padding: compact ? "10px 4px" : undefined }}>{t("sessions.noAgents")}</EmptyState>
        ) : null}
        <label style={{ fontSize: 12, color: "var(--text-2)" }}>
          {t("sessions.col.agent")}
          <select
            ref={agentSelectRef}
            className="z-field"
            aria-label={t("sessions.col.agent")}
            style={{ display: "block", width: "100%", marginTop: 4 }}
            value={agentId}
            disabled={busy || agents.length === 0}
            onChange={(e) => setAgentId(e.target.value)}
          >
            <option value="">{t("sessions.pickAgent")}</option>
            {agents.map((a) => (
              <option key={a.id} value={a.id}>
                {a.display_name || a.agent_key}
              </option>
            ))}
          </select>
        </label>
        <label style={{ fontSize: 12, color: "var(--text-2)" }}>
          {t("sessions.label")}
          <input
            className="z-field"
            style={{ display: "block", width: "100%", marginTop: 4 }}
            placeholder={t("sessions.placeholder.label")}
            value={label}
            disabled={busy}
            autoComplete="off"
            onChange={(e) => setLabel(e.target.value)}
          />
        </label>
        <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
          <Button variant="primary" disabled={busy || loading || agents.length === 0} onClick={() => void create()}>
            {t("sessions.create")}
          </Button>
          <Button
            variant="quiet"
            disabled={busy}
            onClick={() => {
              setCreateOpen(false);
              setFormErr("");
            }}
          >
            {t("sessions.cancelCreate")}
          </Button>
        </div>
      </div>
    );
  }

  function pager() {
    if (!state.showItems || visible.length <= pageSize) return null;
    return (
      <div style={{ display: "flex", gap: 8, alignItems: "center", padding: "10px 16px", fontSize: 12.5, flexWrap: "wrap" }}>
        <Button variant="quiet" disabled={safeOffset === 0} onClick={() => setOffset(Math.max(0, safeOffset - pageSize))}>
          {t("common.prev")}
        </Button>
        <span style={{ color: "var(--text-3)" }}>
          {t("common.page", { from: visible.length ? safeOffset + 1 : 0, to: last, n: visible.length })}
          {" · "}
          {t("common.pageOf", { page: pageNo, pages })}
        </span>
        <Button variant="quiet" disabled={last >= visible.length} onClick={() => setOffset(safeOffset + pageSize)}>
          {t("common.next")}
        </Button>
        <label style={{ display: "flex", gap: 6, alignItems: "center", color: "var(--text-3)", fontSize: 12 }}>
          {t("common.rows")}
          <select
            className="z-field"
            aria-label={t("common.rows")}
            value={pageSize}
            onChange={(e) => {
              setPageSize(Number(e.target.value) || SESSION_PAGE_SIZE);
              setOffset(0);
            }}
          >
            {SESSION_PAGE_SIZES.map((n) => (
              <option key={n} value={n}>
                {n}
              </option>
            ))}
          </select>
        </label>
      </div>
    );
  }

  if (compact) {
    return (
      <div style={{ padding: 12, display: "flex", flexDirection: "column", gap: 8 }} data-chat-list="">
        <div style={{ display: "flex", alignItems: "center", gap: 8, padding: "4px 4px 8px", flexWrap: "wrap" }}>
          <b style={{ fontSize: 13.5, fontWeight: 600, flex: 1 }}>{t("sessions.title")}</b>
          <Button
            variant="primary"
            icon="plus"
            onClick={() => {
              if (createBlocked) return;
              setCreateOpen(true);
            }}
            disabled={createBlocked}
            style={{ padding: "4px 10px" }}
          >
            {t("sessions.newChat")}
          </Button>
          <Button icon="refresh" iconGesture variant="ghost" onClick={() => void load()} style={{ padding: "4px 8px" }}>
            {t("common.refresh")}
          </Button>
        </div>
        {formVisible ? <div style={{ padding: "0 4px 4px" }}>{createFields()}</div> : null}
        <div style={{ padding: "0 4px 4px" }}>{filterBar()}</div>
        <PageStatus kind={state.kind} errorText={errText} staleAt={formatStaleAt(loadedAt, locale)} onReload={() => void load()} />
        {formErr ? <StatusLine kind="error">{formErr}</StatusLine> : null}
        {state.showItems
          ? visible.map((s) => {
              const selected = Boolean(selectedId) && selectedId === s.id;
              const named = sessionDisplayName(s);
              const activity = formatStaleAt(sessionActivityAt(s), locale);
              return (
                <div key={s.id} style={{ display: "flex", alignItems: "stretch", gap: 6 }}>
                  <button
                    type="button"
                    aria-current={selected ? "true" : undefined}
                    onClick={() => onPick(s.id, named)}
                    style={{
                      display: "block",
                      textAlign: "left",
                      background: selected ? "var(--accent-soft)" : "var(--card)",
                      border: `1px solid ${selected ? "var(--accent)" : "var(--border)"}`,
                      borderRadius: 11,
                      padding: "10px 12px",
                      flex: 1,
                      minWidth: 0,
                      transition: "background var(--dur-hover) var(--ease-standard), border-color var(--dur-hover) var(--ease-standard)",
                    }}
                  >
                    <div style={{ fontSize: 13, fontWeight: 600, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                      {named}
                    </div>
                    <div style={{ fontSize: 11.5, color: "var(--text-3)", marginTop: 3 }}>
                      {agentLabel(agents, s.agent_id)}
                      {activity ? ` · ${activity}` : ""}
                    </div>
                    <div style={{ fontSize: 11, color: "var(--text-4)", marginTop: 2 }}>{t("sessions.messagesUnavailable")}</div>
                  </button>
                  <Button
                    variant="ghost"
                    disabled={busy || createBlocked}
                    aria-label={t("sessions.deleteNamed", { name: named })}
                    onClick={() => void remove(s)}
                    style={{ padding: "4px 8px", alignSelf: "center" }}
                  >
                    {t("common.delete")}
                  </Button>
                </div>
              );
            })
          : null}
        {state.showEmpty ? <EmptyState style={{ padding: "16px 8px" }}>{t("sessions.empty")}</EmptyState> : null}
        {filteredEmpty ? <EmptyState style={{ padding: "16px 8px" }}>{t("sessions.emptyFilter")}</EmptyState> : null}
      </div>
    );
  }

  return (
    <PageChrome
      icon="list"
      title={t("sessions.title")}
      description={t("sessions.desc")}
      primary={
        <Button
          variant="primary"
          icon="plus"
          disabled={createBlocked}
          onClick={() => {
            if (createBlocked) return;
            setCreateOpen(true);
            setFormErr("");
          }}
        >
          {t("sessions.newChat")}
        </Button>
      }
      refresh={
        <Button icon="refresh" iconGesture onClick={() => void load()}>
          {t("common.refresh")}
        </Button>
      }
      filters={filterBar()}
    >
      <PageStatus kind={state.kind} errorText={errText} staleAt={formatStaleAt(loadedAt, locale)} onReload={() => void load()} />
      {formErr ? <StatusLine kind="error">{formErr}</StatusLine> : null}
      {formVisible ? (
        <Card>
          <CardHeader icon="plus" title={t("sessions.add")} />
          <div style={{ padding: 14, display: "flex", flexDirection: "column", gap: 10 }}>
            {createFields()}
            {creating ? <StatusLine kind="loading" /> : null}
          </div>
        </Card>
      ) : null}
      <Card>
        <CardHeader icon="msg" title={t("sessions.open")} meta={metaN == null ? "—" : t("sessions.meta", { n: metaN })} />
        <TableScroll>
        <div style={{ display: "flex", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)", gap: 6 }}>
          <span style={{ flex: 2.2 }}>{t("sessions.col.session")}</span>
          <span style={{ flex: 1.6 }}>{t("sessions.col.agent")}</span>
          <span style={{ flex: 1.2 }}>{t("sessions.col.context")}</span>
          <span style={{ flex: 1.2 }}>{t("sessions.col.messages")}</span>
          <span style={{ flex: 1.4 }}>{t("sessions.col.created")}</span>
          <span style={{ flex: 1.2 }}>{t("sessions.col.updated")}</span>
          <span style={{ flex: 1.4 }}>{t("sessions.col.mode")}</span>
          <span style={{ flex: 1.6, textAlign: "right" }}>{t("sessions.col.actions")}</span>
        </div>
        {state.showItems
          ? page.map((s) => {
              const named = sessionDisplayName(s);
              return (
                <div
                  key={s.id}
                  style={{
                    display: "flex",
                    alignItems: "center",
                    padding: "11px 16px",
                    fontSize: 12.5,
                    borderBottom: "1px solid var(--border-soft)",
                    gap: 6,
                  }}
                >
                  <button
                    type="button"
                    onClick={() => onPick(s.id, named)}
                    style={{
                      flex: 2.2,
                      fontWeight: 600,
                      textAlign: "left",
                      background: "transparent",
                      border: "none",
                      color: "var(--text)",
                      cursor: "pointer",
                      padding: 0,
                    }}
                  >
                    {named}
                  </button>
                  <span style={{ flex: 1.6, color: "var(--text-2)" }}>{agentLabel(agents, s.agent_id)}</span>
                  <span style={{ flex: 1.2, color: "var(--text-4)", fontSize: 11.5 }}>{t("sessions.contextUnavailable")}</span>
                  <span style={{ flex: 1.2, color: "var(--text-4)", fontSize: 11.5 }}>{t("sessions.messagesUnavailable")}</span>
                  <span style={{ flex: 1.4, color: "var(--text-3)", fontSize: 12 }}>{formatStaleAt(sessionActivityAt(s), locale) || "—"}</span>
                  <span style={{ flex: 1.2, color: "var(--text-4)", fontSize: 11.5 }}>{t("sessions.updatedUnavailable")}</span>
                  <span style={{ flex: 1.4 }}>
                    <select
                      className="z-field"
                      aria-label={t("sessions.promptMode")}
                      value={normalizePromptMode(s.prompt_mode)}
                      disabled={createBlocked}
                      onChange={(e) => void persistMode(s.id, e.target.value)}
                      style={{ width: "100%" }}
                    >
                      {PROMPT_MODES.map((m) => (
                        <option key={m} value={m}>
                          {t(promptModeKey(m))}
                        </option>
                      ))}
                    </select>
                  </span>
                  <span style={{ flex: 1.6, display: "flex", justifyContent: "flex-end", gap: 8, alignItems: "center" }}>
                    <Button variant="ghost" onClick={() => onPick(s.id, named)} style={{ padding: "4px 8px" }}>
                      {t("sessions.openChat")}
                    </Button>
                    <Button
                      variant="quiet"
                      disabled={busy || createBlocked}
                      aria-label={t("sessions.deleteNamed", { name: named })}
                      onClick={() => void remove(s)}
                      style={{ padding: "4px 8px" }}
                    >
                      {t("common.delete")}
                    </Button>
                  </span>
                </div>
              );
            })
          : null}
        {state.showEmpty ? <EmptyState>{t("sessions.empty")}</EmptyState> : null}
        {filteredEmpty ? <EmptyState>{t("sessions.emptyFilter")}</EmptyState> : null}
        </TableScroll>
        {pager()}
      </Card>
    </PageChrome>
  );
});
