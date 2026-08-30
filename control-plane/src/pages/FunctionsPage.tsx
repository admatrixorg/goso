import { useEffect, useMemo, useRef, useState } from "react";
import { api, type Agent, type Session } from "../api/client";
import { resolveSettled } from "../api/channel-ops";
import {
  classifyToolView,
  confirmNamedTarget,
  cronCreateBlocked,
  filterByQuery,
  parseCronSpec,
  skillNameOk,
  toolViewBlocksMutation,
} from "../api/capabilities-ops";
import { confirmNamed } from "../api/confirm";
import { cronApi, type CronJob } from "../api/cron";
import { classifyPageState, formatStaleAt, inventoryBlocksMutation, isFilteredEmpty, listMetaCount } from "../api/page-state";
import { skillsApi, type SkillInfo } from "../api/skills";
import { toolsApi, type AgentTool } from "../api/tools";
import { useI18n, type MsgKey } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { PageChrome } from "../ui/PageChrome";
import { PageStatus } from "../ui/PageStatus";
import { StatusLine, formatPublicError, redactPublicText } from "../ui/StatusLine";
import { ConnectorPanel } from "./ConnectorPanel";

type FunctionFocus = "skills" | "tools" | "mcp" | "cron";

export function FunctionsPage({ focus = "tools" }: { focus?: FunctionFocus }) {
  if (focus === "skills") return <SkillsSurface />;
  if (focus === "mcp") return <ConnectorPanel variant="mcp" />;
  if (focus === "cron") return <CronSurface />;
  return <ToolsSurface />;
}

const SKILL_GAPS: MsgKey[] = [
  "functions.skills.unavailable.rescan",
  "functions.skills.unavailable.install",
  "functions.skills.unavailable.deps",
  "functions.skills.unavailable.enable",
  "functions.skills.unavailable.edit",
  "functions.skills.unavailable.bulk",
  "functions.skills.unavailable.status",
];

function SkillsSurface() {
  const { t, locale } = useI18n();
  const [rows, setRows] = useState<SkillInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [loaded, setLoaded] = useState(false);
  const [loadedAt, setLoadedAt] = useState<string | null>(null);
  const [err, setErr] = useState<unknown>(null);
  const [actionErr, setActionErr] = useState("");
  const [ok, setOk] = useState("");
  const [query, setQuery] = useState("");
  const [showForm, setShowForm] = useState(false);
  const [skillName, setSkillName] = useState("");
  const [skillBody, setSkillBody] = useState("");
  const req = useRef(0);

  const state = classifyPageState({
    loading,
    loaded,
    error: err,
    itemCount: rows.length,
    keepStale: loaded && rows.length > 0,
  });
  const blocked = inventoryBlocksMutation(state.kind);
  const visible = useMemo(
    () => filterByQuery(state.showItems ? rows : [], query, (s) => `${s.name} ${s.path ?? ""} ${s.snippet ?? ""}`),
    [rows, query, state.showItems],
  );
  const filteredEmpty = Boolean(query.trim()) && state.kind === "empty";
  const trueEmpty = state.showEmpty && !query.trim();
  const metaN = listMetaCount(state.kind, visible.length);

  async function load(q = query) {
    const id = ++req.current;
    setLoading(true);
    try {
      const trimmed = q.trim();
      const j = trimmed ? await skillsApi.search(trimmed) : await skillsApi.list();
      if (id !== req.current) return;
      setRows(j.skills ?? []);
      setLoaded(true);
      setLoadedAt(new Date().toISOString());
      setErr(null);
    } catch (e) {
      if (id !== req.current) return;
      setErr(e);
    } finally {
      if (id === req.current) setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  async function createSkill() {
    if (blocked) return;
    if (!skillNameOk(skillName)) {
      setActionErr(t("functions.skills.needName"));
      return;
    }
    if (!skillBody.trim()) {
      setActionErr(t("functions.skills.needBody"));
      return;
    }
    try {
      const name = skillName.trim();
      await skillsApi.create({ name, body: skillBody });
      setSkillName("");
      setSkillBody("");
      setShowForm(false);
      setOk(t("functions.skills.createdOk", { name }));
      setActionErr("");
      await load();
    } catch (e) {
      setActionErr(formatPublicError(e));
    }
  }

  async function archiveSkill(name: string) {
    if (blocked) return;
    if (!confirmNamed(t("functions.skills.confirmArchive", { name }), (m) => window.confirm(m))) return;
    try {
      await skillsApi.remove(name);
      setOk(t("functions.skills.archivedOk", { name }));
      setActionErr("");
      await load();
    } catch (e) {
      setActionErr(formatPublicError(e));
    }
  }

  return (
    <PageChrome
      icon="doc"
      title={t("functions.skills.title")}
      description={t("functions.skills.desc")}
      primary={
        <Button
          variant="primary"
          icon="plus"
          disabled={blocked}
          onClick={() => {
            if (blocked) return;
            setShowForm(true);
            setActionErr("");
            setOk("");
          }}
        >
          {t("functions.skills.create")}
        </Button>
      }
      refresh={
        <Button icon="refresh" iconGesture onClick={() => void load()} disabled={loading}>
          {t("common.refresh")}
        </Button>
      }
      filters={
        <input
          className="z-field"
          style={{ flex: 1, minWidth: 160 }}
          value={query}
          onChange={(e) => {
            const v = e.target.value;
            setQuery(v);
            void load(v);
          }}
          placeholder={t("functions.skills.search")}
          aria-label={t("functions.skills.search")}
          autoComplete="off"
        />
      }
    >
      <Card>
        <CardHeader icon="lock" title={t("functions.skills.unavailable")} />
        <ul style={{ margin: 0, padding: "0 16px 14px 32px", fontSize: 12.5, color: "var(--text-3)" }}>
          {SKILL_GAPS.map((k) => (
            <li key={k}>{t(k)}</li>
          ))}
        </ul>
      </Card>
      <PageStatus kind={state.kind} errorText={err ? formatPublicError(err) : ""} staleAt={formatStaleAt(loadedAt, locale)} onReload={() => void load()} />
      {actionErr ? <StatusLine kind="error">{actionErr}</StatusLine> : null}
      {ok && !actionErr ? (
        <p role="status" style={{ margin: 0, fontSize: 12.5, color: "var(--green)" }}>
          {ok}
        </p>
      ) : null}
      {!blocked && showForm ? (
        <Card>
          <CardHeader icon="plus" title={t("functions.skills.create")} />
          <div style={{ padding: "0 16px 16px", display: "flex", flexDirection: "column", gap: 10 }}>
            <label style={{ fontSize: 12, color: "var(--text-2)" }}>
              {t("functions.skills.name")}
              <input className="z-field" style={{ display: "block", width: "100%", marginTop: 4 }} value={skillName} onChange={(e) => setSkillName(e.target.value)} autoComplete="off" />
            </label>
            <label style={{ fontSize: 12, color: "var(--text-2)" }}>
              {t("functions.skills.body")}
              <textarea className="z-field" style={{ display: "block", width: "100%", marginTop: 4, minHeight: 64, fontFamily: "var(--font-mono, monospace)" }} value={skillBody} onChange={(e) => setSkillBody(e.target.value)} />
            </label>
            <div style={{ display: "flex", gap: 8 }}>
              <Button variant="primary" onClick={() => void createSkill()}>
                {t("functions.skills.create")}
              </Button>
              <Button variant="quiet" onClick={() => setShowForm(false)}>
                {t("common.cancel")}
              </Button>
            </div>
          </div>
        </Card>
      ) : null}
      <Card>
        <CardHeader icon="doc" title={t("functions.skills")} meta={metaN == null ? "—" : t("functions.skills.meta", { n: metaN })} />
        <TableScroll>
          <div style={{ display: "flex", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)" }}>
            <span style={{ flex: 1.2 }}>{t("functions.skills.col.name")}</span>
            <span style={{ flex: 1.6 }}>{query.trim() ? t("functions.skills.col.snippet") : t("functions.skills.col.path")}</span>
            <span style={{ flex: 0.6 }} />
          </div>
          {state.showItems
            ? visible.map((s) => (
                <div key={s.name} style={{ display: "flex", alignItems: "center", padding: "11px 16px", fontSize: 12.5, borderBottom: "1px solid var(--border-soft)", gap: 8 }}>
                  <span style={{ flex: 1.2, fontWeight: 600 }}>{s.name}</span>
                  <span style={{ flex: 1.6, color: "var(--text-2)" }}>{query.trim() ? s.snippet ?? "" : s.path ?? ""}</span>
                  <span style={{ flex: 0.6, textAlign: "right" }}>
                    <Button variant="ghost" disabled={blocked} onClick={() => void archiveSkill(s.name)}>
                      {t("functions.skills.archive")}
                    </Button>
                  </span>
                </div>
              ))
            : null}
          {trueEmpty ? <EmptyState data-page-state="empty">{t("functions.skills.empty")}</EmptyState> : null}
          {filteredEmpty ? <EmptyState data-page-state="filtered_empty">{t("functions.skills.filterEmpty")}</EmptyState> : null}
        </TableScroll>
      </Card>
    </PageChrome>
  );
}

function ToolsSurface() {
  const { t, locale } = useI18n();
  const [agents, setAgents] = useState<Agent[]>([]);
  const [agentsLoading, setAgentsLoading] = useState(true);
  const [agentsLoaded, setAgentsLoaded] = useState(false);
  const [agentsErr, setAgentsErr] = useState<unknown>(null);
  const [loadedAt, setLoadedAt] = useState<string | null>(null);
  const [agentId, setAgentId] = useState("");
  const [tools, setTools] = useState<AgentTool[]>([]);
  const [toolsLoading, setToolsLoading] = useState(false);
  const [toolsLoaded, setToolsLoaded] = useState(false);
  const [toolsErr, setToolsErr] = useState<unknown>(null);
  const [actionErr, setActionErr] = useState("");
  const [query, setQuery] = useState("");

  const visible = useMemo(
    () => filterByQuery(tools, query, (tool) => `${tool.name} ${tool.connector} ${tool.description ?? ""}`),
    [tools, query],
  );
  const kind = classifyToolView({
    agentsLoading,
    agentsLoaded,
    agentsError: agentsErr,
    agentCount: agents.length,
    agentId,
    toolsLoading,
    toolsLoaded,
    toolsError: toolsErr,
    toolCount: tools.length,
    visibleCount: visible.length,
  });
  const blocked = toolViewBlocksMutation(kind);
  const pageKind = kind === "permission" ? "permission" : kind === "error" ? "error" : kind === "stale" ? "stale" : kind === "loading" ? "loading" : "ready";
  const metaN = kind === "ready" || kind === "filtered_empty" || kind === "stale" ? visible.length : kind === "empty" ? 0 : null;

  async function loadAgents() {
    setAgentsLoading(true);
    try {
      const j = await api.listAgents();
      setAgents(j.agents ?? []);
      setAgentsLoaded(true);
      setLoadedAt(new Date().toISOString());
      setAgentsErr(null);
    } catch (e) {
      setAgentsErr(e);
    } finally {
      setAgentsLoading(false);
    }
  }

  async function loadTools(id: string) {
    if (!id) {
      setTools([]);
      setToolsLoaded(false);
      setToolsErr(null);
      setToolsLoading(false);
      return;
    }
    setToolsLoading(true);
    try {
      const j = await toolsApi.list(id);
      setTools(j.tools ?? []);
      setToolsLoaded(true);
      setLoadedAt(new Date().toISOString());
      setToolsErr(null);
    } catch (e) {
      setToolsErr(e);
    } finally {
      setToolsLoading(false);
    }
  }

  useEffect(() => {
    void loadAgents();
  }, []);

  useEffect(() => {
    void loadTools(agentId);
  }, [agentId]);

  async function toggle(tool: AgentTool) {
    if (blocked || !agentId) return;
    try {
      await toolsApi.setEnabled(agentId, tool.name, !tool.enabled);
      setActionErr("");
      await loadTools(agentId);
    } catch (e) {
      setActionErr(formatPublicError(e));
    }
  }

  return (
    <PageChrome
      icon="build"
      title={t("functions.tools.title")}
      description={t("functions.tools.desc")}
      primary={
        <Button
          icon="refresh"
          iconGesture
          variant="primary"
          onClick={() => {
            void loadAgents();
            void loadTools(agentId);
          }}
          disabled={agentsLoading || toolsLoading}
        >
          {t("common.refresh")}
        </Button>
      }
      filters={
        <>
          <select
            className="z-field"
            value={agentId}
            onChange={(e) => setAgentId(e.target.value)}
            disabled={Boolean(agentsErr) || !agentsLoaded}
            aria-label={t("functions.pickAgent")}
          >
            <option value="">{t("functions.pickAgent")}</option>
            {agents.map((a) => (
              <option key={a.id} value={a.id}>
                {a.display_name || a.agent_key}
              </option>
            ))}
          </select>
          {kind === "ready" || kind === "filtered_empty" || kind === "empty" ? (
            <input
              className="z-field"
              style={{ flex: 1, minWidth: 160 }}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder={t("functions.tools.search")}
              aria-label={t("functions.tools.search")}
              autoComplete="off"
            />
          ) : null}
        </>
      }
    >
      <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("functions.tools.scope")}</p>
      <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("functions.tools.globalUnavailable")}</p>
      <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("functions.workspace.note")}</p>
      {kind === "loading" || kind === "permission" || kind === "error" || kind === "stale" ? (
        <PageStatus
          kind={pageKind}
          errorText={toolsErr ? formatPublicError(toolsErr) : agentsErr ? formatPublicError(agentsErr) : ""}
          staleAt={formatStaleAt(loadedAt, locale)}
          onReload={() => {
            void loadAgents();
            void loadTools(agentId);
          }}
        />
      ) : null}
      {kind === "no_agent" ? (
        <EmptyState data-page-state="dependency">{t("functions.tools.noAgents")}</EmptyState>
      ) : null}
      {kind === "no_selection" ? (
        <EmptyState data-page-state="no_selection">{t("functions.emptyAgent")}</EmptyState>
      ) : null}
      {kind === "unsupported" ? (
        <EmptyState data-page-state="unsupported">{t("functions.tools.unsupported")}</EmptyState>
      ) : null}
      {actionErr ? <StatusLine kind="error">{actionErr}</StatusLine> : null}
      <Card>
        <CardHeader icon="build" title={t("functions.tools")} meta={metaN == null ? "—" : t("functions.meta", { n: metaN })} />
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
            }}
          >
            <span style={{ flex: 1.5 }}>{t("functions.col.name")}</span>
            <span style={{ flex: 1.1 }}>{t("functions.col.connector")}</span>
            <span style={{ flex: 0.9 }}>{t("functions.col.approval")}</span>
            <span style={{ flex: 1 }}>{t("functions.col.configured")}</span>
            <span style={{ flex: 0.9 }}>{t("functions.col.granted")}</span>
            <span style={{ flex: 0.8, textAlign: "right" }}>{t("functions.col.on")}</span>
          </div>
          {kind === "ready" || kind === "stale"
            ? visible.map((tool) => (
                <div
                  key={`${tool.connector}:${tool.name}`}
                  style={{ display: "flex", alignItems: "center", padding: "11px 16px", fontSize: 12.5, borderBottom: "1px solid var(--border-soft)" }}
                >
                  <span style={{ flex: 1.5, fontWeight: 600 }}>{tool.name}</span>
                  <span style={{ flex: 1.1, color: "var(--text-2)" }}>{tool.connector}</span>
                  <span style={{ flex: 0.9 }}>
                    <Badge tone={tool.requires_approval ? "warning" : "neutral"}>
                      {tool.requires_approval ? t("functions.approvalYes") : t("functions.approvalNo")}
                    </Badge>
                  </span>
                  <span style={{ flex: 1 }}>
                    <Badge tone={tool.configured ? "positive" : "neutral"}>
                      {tool.configured ? t("functions.configured") : t("functions.notConfigured")}
                    </Badge>
                  </span>
                  <span style={{ flex: 0.9 }}>
                    <Badge tone={tool.granted !== false ? "positive" : "neutral"}>
                      {tool.granted !== false ? t("functions.granted") : t("functions.ungranted")}
                    </Badge>
                  </span>
                  <span style={{ flex: 0.8, textAlign: "right" }}>
                    <Button variant={tool.enabled ? "primary" : "ghost"} disabled={blocked} onClick={() => void toggle(tool)}>
                      {tool.enabled ? t("common.enabled") : t("common.disabled")}
                    </Button>
                  </span>
                </div>
              ))
            : null}
          {kind === "empty" ? <EmptyState data-page-state="empty">{t("functions.empty")}</EmptyState> : null}
          {kind === "filtered_empty" ? <EmptyState data-page-state="filtered_empty">{t("functions.tools.filterEmpty")}</EmptyState> : null}
        </TableScroll>
      </Card>
    </PageChrome>
  );
}

function CronSurface() {
  const { t, locale } = useI18n();
  const [jobs, setJobs] = useState<CronJob[]>([]);
  const [sessions, setSessions] = useState<Session[]>([]);
  const [loading, setLoading] = useState(true);
  const [loaded, setLoaded] = useState(false);
  const [loadedAt, setLoadedAt] = useState<string | null>(null);
  const [err, setErr] = useState<unknown>(null);
  const [sessionsErr, setSessionsErr] = useState<unknown>(null);
  const [sessionsLoaded, setSessionsLoaded] = useState(false);
  const [actionErr, setActionErr] = useState("");
  const [ok, setOk] = useState("");
  const [query, setQuery] = useState("");
  const [showForm, setShowForm] = useState(false);
  const [cronSpec, setCronSpec] = useState("every:1h");
  const [cronSession, setCronSession] = useState("");
  const [cronMessage, setCronMessage] = useState("");

  const jobsState = classifyPageState({
    loading,
    loaded,
    error: err,
    itemCount: jobs.length,
    keepStale: loaded && jobs.length > 0,
  });
  const sessState = classifyPageState({
    loading: false,
    loaded: sessionsLoaded,
    error: sessionsErr,
    itemCount: sessions.length,
  });
  const blocked = inventoryBlocksMutation(jobsState.kind);
  const createBlocked = cronCreateBlocked(jobsState, sessState, sessions.length);
  const visible = useMemo(
    () => filterByQuery(jobsState.showItems ? jobs : [], query, (j) => `${j.spec} ${j.session_id} ${j.message}`),
    [jobs, query, jobsState.showItems],
  );
  const filteredEmpty = isFilteredEmpty(jobsState, jobs.length, visible.length);
  const metaN = listMetaCount(jobsState.kind, visible.length);

  async function load() {
    setLoading(true);
    const [jobsRes, sessRes] = await Promise.allSettled([cronApi.list(), api.listSessions()]);
    const j = resolveSettled(jobsRes);
    if (j.ok) {
      setJobs(j.value.jobs ?? []);
      setLoaded(true);
      setLoadedAt(new Date().toISOString());
      setErr(null);
    } else {
      setErr(j.error);
    }
    const s = resolveSettled(sessRes);
    if (s.ok) {
      setSessions(s.value.sessions ?? []);
      setSessionsLoaded(true);
      setSessionsErr(null);
    } else {
      setSessionsErr(s.error);
      setSessionsLoaded(false);
    }
    setLoading(false);
  }

  useEffect(() => {
    void load();
  }, []);

  async function createCron() {
    if (createBlocked) return;
    const parsed = parseCronSpec(cronSpec);
    if (!parsed.ok) {
      setActionErr(parsed.reason === "once" ? t("functions.cron.onceUnavailable") : parsed.reason === "empty" ? t("functions.cron.needSpec") : t("functions.cron.invalidSpec"));
      return;
    }
    if (!cronSession.trim()) {
      setActionErr(t("functions.cron.needSession"));
      return;
    }
    if (!cronMessage.trim()) {
      setActionErr(t("functions.cron.needMessage"));
      return;
    }
    try {
      await cronApi.create({ spec: cronSpec.trim(), session_id: cronSession.trim(), message: cronMessage.trim() });
      setCronMessage("");
      setShowForm(false);
      setOk(t("functions.cron.createdOk"));
      setActionErr("");
      await load();
    } catch (e) {
      setActionErr(formatPublicError(e));
    }
  }

  async function toggleCron(job: CronJob) {
    if (blocked) return;
    try {
      await cronApi.setEnabled(job.id, !(job.enabled !== false));
      setActionErr("");
      await load();
    } catch (e) {
      setActionErr(formatPublicError(e));
    }
  }

  async function deleteCron(job: CronJob) {
    if (blocked) return;
    const name = job.spec || job.id;
    if (!confirmNamedTarget(t("functions.cron.confirmDeleteNamed", { name }), (m) => window.confirm(m))) return;
    try {
      await cronApi.remove(job.id);
      setOk(t("functions.cron.deletedOk", { name }));
      setActionErr("");
      await load();
    } catch (e) {
      setActionErr(formatPublicError(e));
    }
  }

  return (
    <PageChrome
      icon="timer"
      title={t("functions.cron.title")}
      description={t("functions.cron.desc")}
      primary={
        <Button
          variant="primary"
          icon="plus"
          disabled={createBlocked}
          onClick={() => {
            if (createBlocked) return;
            setShowForm(true);
            setActionErr("");
            setOk("");
          }}
        >
          {t("functions.cron.create")}
        </Button>
      }
      refresh={
        <Button icon="refresh" iconGesture onClick={() => void load()} disabled={loading}>
          {t("common.refresh")}
        </Button>
      }
      filters={
        jobsState.showItems || jobsState.showEmpty ? (
          <input
            className="z-field"
            style={{ flex: 1, minWidth: 160 }}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={t("functions.cron.search")}
            aria-label={t("functions.cron.search")}
            autoComplete="off"
          />
        ) : null
      }
    >
      <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("functions.cron.onceUnavailable")}</p>
      <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("functions.cron.agentTargetUnavailable")}</p>
      <PageStatus kind={jobsState.kind} errorText={err ? formatPublicError(err) : ""} staleAt={formatStaleAt(loadedAt, locale)} onReload={() => void load()} />
      {sessionsErr ? (
        <div data-page-state="dependency">
          <StatusLine kind="error">
            {t("functions.cron.sessionsFailed")} · {formatPublicError(sessionsErr)}
          </StatusLine>
        </div>
      ) : null}
      {sessState.showEmpty && !sessionsErr ? (
        <EmptyState data-page-state="dependency">{t("functions.cron.noSessions")}</EmptyState>
      ) : null}
      {actionErr ? <StatusLine kind="error">{actionErr}</StatusLine> : null}
      {ok && !actionErr ? (
        <p role="status" style={{ margin: 0, fontSize: 12.5, color: "var(--green)" }}>
          {ok}
        </p>
      ) : null}
      {!createBlocked && showForm ? (
        <Card>
          <CardHeader icon="plus" title={t("functions.cron.create")} />
          <div style={{ padding: "0 16px 16px", display: "flex", flexDirection: "column", gap: 10 }}>
            <label style={{ fontSize: 12, color: "var(--text-2)" }}>
              {t("functions.cron.spec")}
              <input className="z-field" style={{ display: "block", width: "100%", marginTop: 4 }} value={cronSpec} onChange={(e) => setCronSpec(e.target.value)} autoComplete="off" />
            </label>
            <label style={{ fontSize: 12, color: "var(--text-2)" }}>
              {t("functions.cron.session")}
              <select className="z-field" style={{ display: "block", width: "100%", marginTop: 4 }} value={cronSession} onChange={(e) => setCronSession(e.target.value)}>
                <option value="">{t("functions.cron.pickSession")}</option>
                {sessions.map((s) => (
                  <option key={s.id} value={s.id}>
                    {s.label || s.id}
                  </option>
                ))}
              </select>
            </label>
            <label style={{ fontSize: 12, color: "var(--text-2)" }}>
              {t("functions.cron.message")}
              <input className="z-field" style={{ display: "block", width: "100%", marginTop: 4 }} value={cronMessage} onChange={(e) => setCronMessage(e.target.value)} autoComplete="off" />
            </label>
            <div style={{ display: "flex", gap: 8 }}>
              <Button variant="primary" onClick={() => void createCron()}>
                {t("functions.cron.create")}
              </Button>
              <Button variant="quiet" onClick={() => setShowForm(false)}>
                {t("common.cancel")}
              </Button>
            </div>
          </div>
        </Card>
      ) : null}
      <Card>
        <CardHeader icon="timer" title={t("functions.cron")} meta={metaN == null ? "—" : t("functions.cron.meta", { n: metaN })} />
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
            }}
          >
            <span style={{ flex: 1.1 }}>{t("functions.cron.col.spec")}</span>
            <span style={{ flex: 1.1 }}>{t("functions.cron.col.session")}</span>
            <span style={{ flex: 1.3 }}>{t("functions.cron.col.message")}</span>
            <span style={{ flex: 0.9 }}>{t("functions.cron.col.last")}</span>
            <span style={{ flex: 1.1 }}>{t("functions.cron.col.error")}</span>
            <span style={{ flex: 0.7 }}>{t("functions.cron.col.enabled")}</span>
            <span style={{ flex: 0.6 }} />
          </div>
          {jobsState.showItems
            ? visible.map((job) => (
                <div key={job.id} style={{ display: "flex", alignItems: "center", padding: "11px 16px", fontSize: 12.5, borderBottom: "1px solid var(--border-soft)", gap: 8 }}>
                  <span style={{ flex: 1.1, fontWeight: 600 }}>{job.spec}</span>
                  <span style={{ flex: 1.1, color: "var(--text-2)" }}>{job.session_id}</span>
                  <span style={{ flex: 1.3, color: "var(--text-2)" }}>{job.message}</span>
                  <span style={{ flex: 0.9, color: "var(--text-3)" }}>{job.last_run || "—"}</span>
                  <span style={{ flex: 1.1, color: "var(--red)", fontSize: 11 }}>{job.last_error ? redactPublicText(job.last_error) : "—"}</span>
                  <span style={{ flex: 0.7 }}>
                    <Button variant={job.enabled !== false ? "primary" : "ghost"} disabled={blocked} onClick={() => void toggleCron(job)}>
                      {job.enabled !== false ? t("common.enabled") : t("common.disabled")}
                    </Button>
                  </span>
                  <span style={{ flex: 0.6, textAlign: "right" }}>
                    <Button variant="ghost" disabled={blocked} onClick={() => void deleteCron(job)}>
                      {t("common.delete")}
                    </Button>
                  </span>
                </div>
              ))
            : null}
          {jobsState.showEmpty ? <EmptyState data-page-state="empty">{t("functions.cron.empty")}</EmptyState> : null}
          {filteredEmpty ? <EmptyState data-page-state="filtered_empty">{t("functions.cron.filterEmpty")}</EmptyState> : null}
        </TableScroll>
      </Card>
    </PageChrome>
  );
}
