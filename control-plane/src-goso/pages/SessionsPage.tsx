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
  searchConversations,
  sessionDisplayName,
} from "../api/sessions";
import { useI18n, type MsgKey } from "../i18n";
import { Button } from "../ui/Button";
import { Icon } from "../ui/Icon";
import { Card, CardHeader } from "../ui/Card";
import { DataTable, THead, Th, Td, Tr } from "../ui/DataTable";
import { EmptyState } from "../ui/EmptyState";
import { PageChrome } from "../ui/PageChrome";
import { PageStatus } from "../ui/PageStatus";
import { SearchInput } from "../ui/SearchInput";
import { StatusLine, formatPublicError } from "../ui/StatusLine";
import { fieldClass, selectClass } from "../ui/field";

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
    onRenamed?: (id: string, label: string) => void;
  }
>(function SessionsPage({ onPick, compact, selectedId, onDeleted, onRenamed }, ref) {
  const { t, locale } = useI18n();
  function titleOf(s: { label?: string; preview?: string }) {
    return sessionDisplayName(s, t("chat.untitled"));
  }
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
  const [menuFor, setMenuFor] = useState("");
  const [renamingId, setRenamingId] = useState("");
  const [renameValue, setRenameValue] = useState("");
  const [showArchived, setShowArchived] = useState(false);
  const [searchOpen, setSearchOpen] = useState(false);
  const [searchQ, setSearchQ] = useState("");
  const [searchIdx, setSearchIdx] = useState(0);
  const searchRef = useRef<HTMLInputElement>(null);
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

  useEffect(() => {
    if (!compact) return;
    const open = () => {
      setSearchOpen((v) => !v);
      setSearchQ("");
      setSearchIdx(0);
    };
    window.addEventListener("goso-chat-search", open);
    return () => window.removeEventListener("goso-chat-search", open);
  }, [compact]);

  useEffect(() => {
    if (searchOpen) searchRef.current?.focus();
  }, [searchOpen]);

  useEffect(() => {
    if (!menuFor) return;
    const close = (e: PointerEvent) => {
      const node = e.target as HTMLElement | null;
      if (node?.closest?.(".z-gs-menu, .z-gs-more")) return;
      setMenuFor("");
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setMenuFor("");
    };
    window.addEventListener("pointerdown", close);
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("pointerdown", close);
      window.removeEventListener("keydown", onKey);
    };
  }, [menuFor]);

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
      onPick(created.id, titleOf(created));
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

  async function rename(id: string) {
    const next = renameValue.trim();
    if (!next || busy) return;
    try {
      const updated = await api.updateSession(id, { label: next });
      setSessions((prev) => prev.map((row) => (row.id === id ? { ...row, label: updated.label || next } : row)));
      onRenamed?.(id, updated.label || next);
      setRenamingId("");
      setMenuFor("");
      setFormErr("");
    } catch (e) {
      setFormErr(formatPublicError(e));
    }
  }

  async function setArchived(s: Session, archived: boolean) {
    if (busy) return;
    try {
      const updated = await api.updateSession(s.id, { archived });
      setSessions((prev) => prev.map((row) => (row.id === s.id ? { ...row, archived: Boolean(updated.archived) } : row)));
      setMenuFor("");
      setFormErr("");
      if (archived) setShowArchived(true);
    } catch (e) {
      setFormErr(formatPublicError(e));
    }
  }

  async function remove(s: Session) {
    if (busy || createBlocked) return;
    const named = titleOf(s);
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
      <div className="flex flex-wrap items-center gap-2">
        <SearchInput
          value={query}
          onChange={(v) => {
            setQuery(v);
            setOffset(0);
          }}
          placeholder={t("sessions.search")}
          aria-label={t("sessions.search")}
        />
        <select
          className={selectClass}
          aria-label={t("sessions.filterAgent")}
          value={filterAgent}
          onChange={(e) => {
            setFilterAgent(e.target.value);
            setOffset(0);
          }}
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
          <EmptyState icon="bot" title={t("sessions.noAgents")} className={compact ? "py-4" : undefined} />
        ) : null}
        <label className="text-sm font-medium">
          {t("sessions.col.agent")}
          <select
            ref={agentSelectRef}
            className={selectClass + " mt-1 w-full"}
            aria-label={t("sessions.col.agent")}
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
        <label className="text-sm font-medium">
          {t("sessions.label")}
          <input
            className={fieldClass + " mt-1"}
            placeholder={t("sessions.placeholder.label")}
            value={label}
            disabled={busy}
            autoComplete="off"
            onChange={(e) => setLabel(e.target.value)}
          />
        </label>
        <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
          <Button variant="default" disabled={busy || loading || agents.length === 0} onClick={() => void create()}>
            {t("sessions.create")}
          </Button>
          <Button
            variant="outline"
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
        <Button variant="outline" size="sm" disabled={safeOffset === 0} onClick={() => setOffset(Math.max(0, safeOffset - pageSize))}>
          {t("common.prev")}
        </Button>
        <span style={{ color: "var(--text-3)" }}>
          {t("common.page", { from: visible.length ? safeOffset + 1 : 0, to: last, n: visible.length })}
          {" · "}
          {t("common.pageOf", { page: pageNo, pages })}
        </span>
        <Button variant="outline" size="sm" disabled={last >= visible.length} onClick={() => setOffset(safeOffset + pageSize)}>
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
    const recent = sessions.filter((s) => !s.archived);
    const archived = sessions.filter((s) => s.archived);
    const hits = searchConversations(sessions, searchQ, (id) => agentLabel(agents, id));
    const row = (s: Session) => {
      const selected = Boolean(selectedId) && selectedId === s.id;
      const named = titleOf(s);
      return (
        <div key={s.id} className={selected ? "z-gs-cvw is-on" : "z-gs-cvw"}>
          {renamingId === s.id ? (
            <input
              className="z-gs-rename"
              aria-label={t("chat.rename")}
              value={renameValue}
              placeholder={t("chat.renamePlaceholder")}
              onChange={(e) => setRenameValue(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") void rename(s.id);
                if (e.key === "Escape") setRenamingId("");
              }}
            />
          ) : (
            <button
              type="button"
              className="z-gs-cv"
              aria-current={selected ? "true" : undefined}
              onClick={() => onPick(s.id, named)}
            >
              {named}
            </button>
          )}
          <button
            type="button"
            className={menuFor === s.id ? "z-gs-more is-open" : "z-gs-more"}
            aria-label={t("chat.more")}
            aria-expanded={menuFor === s.id}
            onClick={() => setMenuFor((cur) => (cur === s.id ? "" : s.id))}
          >
            ···
          </button>
          {menuFor === s.id ? (
            <div className="z-gs-menu" role="menu">
              <button
                type="button"
                onClick={() => {
                  setRenamingId(s.id);
                  setRenameValue(s.label || "");
                  setMenuFor("");
                }}
              >
                {t("chat.rename")}
              </button>
              <button type="button" disabled title={t("chat.projectLater")}>
                {t("chat.addProject")}
              </button>
              {s.archived ? (
                <button type="button" onClick={() => void setArchived(s, false)}>
                  {t("chat.restore")}
                </button>
              ) : (
                <button type="button" onClick={() => void setArchived(s, true)}>
                  {t("chat.archive")}
                </button>
              )}
              <button type="button" className="is-danger" onClick={() => void remove(s)}>
                {t("common.delete")}
              </button>
            </div>
          ) : null}
        </div>
      );
    };
    return (
      <div className="z-gs-sb" data-chat-list="">
        <div className="z-gs-top">
          <button
            type="button"
            className="z-gs-new"
            onClick={() => {
              if (createBlocked) return;
              setCreateOpen(true);
            }}
            disabled={createBlocked}
          >
            {t("chat.newConv")}
          </button>
          <button
            type="button"
            className="z-gs-ic"
            aria-label={t("chat.search")}
            title={t("chat.searchHint")}
            onClick={() => {
              setSearchOpen(true);
              setSearchQ("");
              setSearchIdx(0);
            }}
          >
            <Icon name="search" size={16} />
          </button>
        </div>
        {formVisible ? <div className="z-gs-form">{createFields()}</div> : null}
        {state.kind === "error" || state.kind === "permission" ? <p className="z-gs-note">{errText}</p> : null}
        {formErr ? <p className="z-gs-note">{formErr}</p> : null}
        {state.showItems && recent.length ? <h6>{t("chat.recent")}</h6> : null}
        {state.showItems ? recent.map(row) : null}
        {archived.length ? (
          <button type="button" className="z-gs-arch" onClick={() => setShowArchived((v) => !v)}>
            {showArchived ? t("chat.hideArchived") : t("chat.archivedCount", { n: archived.length })}
          </button>
        ) : null}
        {showArchived && archived.length ? <h6>{t("chat.archivedHead")}</h6> : null}
        {showArchived ? archived.map(row) : null}
        {state.showEmpty ? <p className="z-gs-note">{t("sessions.empty")}</p> : null}
        {searchOpen ? (
          <div className="z-gs-search" role="dialog" aria-label={t("chat.search")}>
            <input
              ref={searchRef}
              className="z-field"
              value={searchQ}
              placeholder={t("chat.searchPlaceholder")}
              aria-label={t("chat.search")}
              onChange={(e) => {
                setSearchQ(e.target.value);
                setSearchIdx(0);
              }}
              onKeyDown={(e) => {
                if (e.key === "Escape") {
                  setSearchOpen(false);
                  return;
                }
                if (e.key === "ArrowDown") {
                  e.preventDefault();
                  setSearchIdx((i) => Math.min(hits.length - 1, i + 1));
                } else if (e.key === "ArrowUp") {
                  e.preventDefault();
                  setSearchIdx((i) => Math.max(0, i - 1));
                } else if (e.key === "Enter" && hits[searchIdx]) {
                  e.preventDefault();
                  const hit = hits[searchIdx];
                  onPick(hit.id, titleOf(hit));
                  if (hit.archived) setShowArchived(true);
                  setSearchOpen(false);
                }
              }}
            />
            <div className="z-gs-search-list">
              {hits.length === 0 ? (
                <p className="z-gs-note">{t("chat.searchEmpty")}</p>
              ) : (
                hits.map((hit, i) => (
                  <button
                    key={hit.id}
                    type="button"
                    className={i === searchIdx ? "is-on" : ""}
                    onMouseEnter={() => setSearchIdx(i)}
                    onClick={() => {
                      onPick(hit.id, titleOf(hit));
                      if (hit.archived) setShowArchived(true);
                      setSearchOpen(false);
                    }}
                  >
                    <b>{titleOf(hit)}</b>
                    <small>
                      {hit.archived ? t("chat.whereArchived") : t("chat.whereRecent")}
                      {" · "}
                      {agentLabel(agents, hit.agent_id)}
                    </small>
                  </button>
                ))
              )}
            </div>
            <button type="button" className="z-gs-arch" onClick={() => setSearchOpen(false)}>
              {t("common.cancel")}
            </button>
          </div>
        ) : null}
      </div>
    );
  }

  return (
    <PageChrome
      icon="history"
      title={t("sessions.title")}
      description={t("sessions.desc")}
      primary={
        <Button
          variant="default"
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
        <Button variant="outline" icon="refresh" iconGesture onClick={() => void load()}>
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
          <div className="flex flex-col gap-3 p-4">
            {createFields()}
            {creating ? <StatusLine kind="loading" /> : null}
          </div>
        </Card>
      ) : null}
      {state.showEmpty ? <EmptyState icon="history" title={t("sessions.empty")} /> : null}
      {filteredEmpty ? <EmptyState icon="search" title={t("sessions.emptyFilter")} /> : null}
      {state.showItems ? (
        <DataTable minWidth="750px">
          <THead>
            <Th>{t("sessions.col.session")}</Th>
            <Th>{t("sessions.col.agent")}</Th>
            <Th>{t("sessions.col.context")}</Th>
            <Th className="text-right">{t("sessions.col.messages")}</Th>
            <Th>{t("sessions.col.created")}</Th>
            <Th>{t("sessions.col.updated")}</Th>
            <Th>{t("sessions.col.mode")}</Th>
            <Th className="text-right">{t("sessions.col.actions")}</Th>
          </THead>
          <tbody>
            {page.map((s) => {
              const named = titleOf(s);
              return (
                <Tr key={s.id} onClick={() => onPick(s.id, named)}>
                  <Td className="font-semibold">{named}</Td>
                  <Td className="text-muted-foreground">{agentLabel(agents, s.agent_id)}</Td>
                  <Td className="text-xs text-muted-foreground">{t("sessions.contextUnavailable")}</Td>
                  <Td className="text-right text-xs text-muted-foreground">{t("sessions.messagesUnavailable")}</Td>
                  <Td className="text-muted-foreground">{formatStaleAt(sessionActivityAt(s), locale) || "—"}</Td>
                  <Td className="text-xs text-muted-foreground">{t("sessions.updatedUnavailable")}</Td>
                  <Td onClick={(e) => e.stopPropagation()}>
                    <select
                      className={selectClass}
                      aria-label={t("sessions.promptMode")}
                      value={normalizePromptMode(s.prompt_mode)}
                      disabled={createBlocked}
                      onChange={(e) => void persistMode(s.id, e.target.value)}
                    >
                      {PROMPT_MODES.map((m) => (
                        <option key={m} value={m}>
                          {t(promptModeKey(m))}
                        </option>
                      ))}
                    </select>
                  </Td>
                  <Td onClick={(e) => e.stopPropagation()}>
                    <div className="flex justify-end gap-2">
                      <Button variant="ghost" size="sm" onClick={() => onPick(s.id, named)}>
                        {t("sessions.openChat")}
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="text-destructive"
                        disabled={busy || createBlocked}
                        aria-label={t("sessions.deleteNamed", { name: named })}
                        onClick={() => void remove(s)}
                      >
                        {t("common.delete")}
                      </Button>
                    </div>
                  </Td>
                </Tr>
              );
            })}
          </tbody>
        </DataTable>
      ) : null}
      {pager()}
    </PageChrome>
  );
});
