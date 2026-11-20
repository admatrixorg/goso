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
import { Button } from "../ui/Button";
import { Card, CardHeader } from "../ui/Card";
import { DataTable, THead, Th, Td, Tr } from "../ui/DataTable";
import { EmptyState } from "../ui/EmptyState";
import { PageChrome } from "../ui/PageChrome";
import { PageStatus } from "../ui/PageStatus";
import { SearchInput } from "../ui/SearchInput";
import { StatusBadge } from "../ui/StatusBadge";
import { StatusLine, formatPublicError, redactPublicText } from "../ui/StatusLine";
import { fieldClass, labelClass, selectClass } from "../ui/field";
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
      icon="sparkles"
      title={t("functions.skills.title")}
      description={t("functions.skills.desc")}
      primary={
        <Button
          variant="default"
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
        <Button variant="outline" icon="refresh" iconGesture onClick={() => void load()} disabled={loading}>
          {t("common.refresh")}
        </Button>
      }
      filters={
        <SearchInput
          value={query}
          onChange={(v) => {
            setQuery(v);
            void load(v);
          }}
          placeholder={t("functions.skills.search")}
          aria-label={t("functions.skills.search")}
        />
      }
    >
      <Card>
        <CardHeader icon="lock" title={t("functions.skills.unavailable")} />
        <ul className="px-4 pb-4 pl-8 text-sm text-muted-foreground">
          {SKILL_GAPS.map((k) => (
            <li key={k}>{t(k)}</li>
          ))}
        </ul>
      </Card>
      <PageStatus kind={state.kind} errorText={err ? formatPublicError(err) : ""} staleAt={formatStaleAt(loadedAt, locale)} onReload={() => void load()} />
      {actionErr ? <StatusLine kind="error">{actionErr}</StatusLine> : null}
      {ok && !actionErr ? (
        <p role="status" className="m-0 text-sm text-success">
          {ok}
        </p>
      ) : null}
      {!blocked && showForm ? (
        <Card>
          <CardHeader icon="plus" title={t("functions.skills.create")} />
          <div className="flex flex-col gap-3 p-4">
            <label className={labelClass}>
              {t("functions.skills.name")}
              <input className={fieldClass + " mt-1"} value={skillName} onChange={(e) => setSkillName(e.target.value)} autoComplete="off" />
            </label>
            <label className={labelClass}>
              {t("functions.skills.body")}
              <textarea
                className={fieldClass + " mt-1 min-h-16 font-mono"}
                value={skillBody}
                onChange={(e) => setSkillBody(e.target.value)}
              />
            </label>
            <div className="flex gap-2">
              <Button variant="default" onClick={() => void createSkill()}>
                {t("functions.skills.create")}
              </Button>
              <Button variant="outline" onClick={() => setShowForm(false)}>
                {t("common.cancel")}
              </Button>
            </div>
          </div>
        </Card>
      ) : null}
      {trueEmpty ? <EmptyState icon="bolt" title={t("functions.skills.empty")} data-page-state="empty" /> : null}
      {filteredEmpty ? <EmptyState icon="search" title={t("functions.skills.filterEmpty")} data-page-state="filtered_empty" /> : null}
      {state.showItems || trueEmpty || filteredEmpty ? (
        <p className="m-0 text-xs text-muted-foreground">{metaN == null ? "—" : t("functions.skills.meta", { n: metaN })}</p>
      ) : null}
      {state.showItems ? (
        <DataTable minWidth="640px">
          <THead>
            <Th>{t("functions.skills.col.name")}</Th>
            <Th>{query.trim() ? t("functions.skills.col.snippet") : t("functions.skills.col.path")}</Th>
            <Th className="text-right" />
          </THead>
          <tbody>
            {visible.map((s) => (
              <Tr key={s.name}>
                <Td className="font-semibold">{s.name}</Td>
                <Td className="text-muted-foreground">{query.trim() ? s.snippet ?? "" : s.path ?? ""}</Td>
                <Td className="text-right" onClick={(e) => e.stopPropagation()}>
                  <Button variant="ghost" size="sm" className="text-destructive" disabled={blocked} onClick={() => void archiveSkill(s.name)}>
                    {t("functions.skills.archive")}
                  </Button>
                </Td>
              </Tr>
            ))}
          </tbody>
        </DataTable>
      ) : null}
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
      icon="wrench"
      title={t("functions.tools.title")}
      description={t("functions.tools.desc")}
      refresh={
        <Button
          icon="refresh"
          iconGesture
          variant="outline"
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
            className={selectClass}
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
            <SearchInput
              value={query}
              onChange={setQuery}
              placeholder={t("functions.tools.search")}
              aria-label={t("functions.tools.search")}
            />
          ) : null}
        </>
      }
    >
      <p className="m-0 text-sm text-muted-foreground">{t("functions.tools.scope")}</p>
      <p className="m-0 text-sm text-muted-foreground">{t("functions.tools.globalUnavailable")}</p>
      <p className="m-0 text-sm text-muted-foreground">{t("functions.workspace.note")}</p>
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
        <EmptyState icon="bolt" title={t("functions.tools.noAgents")} data-page-state="dependency" />
      ) : null}
      {kind === "no_selection" ? (
        <EmptyState icon="bolt" title={t("functions.emptyAgent")} data-page-state="no_selection" />
      ) : null}
      {kind === "unsupported" ? (
        <EmptyState icon="bolt" title={t("functions.tools.unsupported")} data-page-state="unsupported" />
      ) : null}
      {actionErr ? <StatusLine kind="error">{actionErr}</StatusLine> : null}
      {kind === "empty" ? <EmptyState icon="bolt" title={t("functions.empty")} data-page-state="empty" /> : null}
      {kind === "filtered_empty" ? <EmptyState icon="search" title={t("functions.tools.filterEmpty")} data-page-state="filtered_empty" /> : null}
      {kind === "ready" || kind === "stale" || kind === "empty" || kind === "filtered_empty" ? (
        <p className="m-0 text-xs text-muted-foreground">{metaN == null ? "—" : t("functions.meta", { n: metaN })}</p>
      ) : null}
      {kind === "ready" || kind === "stale" ? (
        <DataTable minWidth="800px">
          <THead>
            <Th>{t("functions.col.name")}</Th>
            <Th>{t("functions.col.connector")}</Th>
            <Th>{t("functions.col.approval")}</Th>
            <Th>{t("functions.col.configured")}</Th>
            <Th>{t("functions.col.granted")}</Th>
            <Th className="text-right">{t("functions.col.on")}</Th>
          </THead>
          <tbody>
            {visible.map((tool) => (
              <Tr key={`${tool.connector}:${tool.name}`}>
                <Td className="font-semibold">{tool.name}</Td>
                <Td className="text-muted-foreground">{tool.connector}</Td>
                <Td>
                  <StatusBadge
                    status={tool.requires_approval ? "warning" : "default"}
                    label={tool.requires_approval ? t("functions.approvalYes") : t("functions.approvalNo")}
                  />
                </Td>
                <Td>
                  <StatusBadge
                    status={tool.configured ? "success" : "default"}
                    label={tool.configured ? t("functions.configured") : t("functions.notConfigured")}
                  />
                </Td>
                <Td>
                  <StatusBadge
                    status={tool.granted !== false ? "success" : "default"}
                    label={tool.granted !== false ? t("functions.granted") : t("functions.ungranted")}
                  />
                </Td>
                <Td className="text-right" onClick={(e) => e.stopPropagation()}>
                  <Button variant={tool.enabled ? "default" : "outline"} size="sm" disabled={blocked} onClick={() => void toggle(tool)}>
                    {tool.enabled ? t("common.enabled") : t("common.disabled")}
                  </Button>
                </Td>
              </Tr>
            ))}
          </tbody>
        </DataTable>
      ) : null}
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
      icon="calendar-clock"
      title={t("functions.cron.title")}
      description={t("functions.cron.desc")}
      primary={
        <Button
          variant="default"
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
        <Button variant="outline" icon="refresh" iconGesture onClick={() => void load()} disabled={loading}>
          {t("common.refresh")}
        </Button>
      }
      filters={
        jobsState.showItems || jobsState.showEmpty ? (
          <SearchInput
            value={query}
            onChange={setQuery}
            placeholder={t("functions.cron.search")}
            aria-label={t("functions.cron.search")}
          />
        ) : null
      }
    >
      <p className="m-0 text-sm text-muted-foreground">{t("functions.cron.onceUnavailable")}</p>
      <p className="m-0 text-sm text-muted-foreground">{t("functions.cron.agentTargetUnavailable")}</p>
      <PageStatus kind={jobsState.kind} errorText={err ? formatPublicError(err) : ""} staleAt={formatStaleAt(loadedAt, locale)} onReload={() => void load()} />
      {sessionsErr ? (
        <div data-page-state="dependency">
          <StatusLine kind="error">
            {t("functions.cron.sessionsFailed")} · {formatPublicError(sessionsErr)}
          </StatusLine>
        </div>
      ) : null}
      {sessState.showEmpty && !sessionsErr ? (
        <EmptyState icon="history" title={t("functions.cron.noSessions")} data-page-state="dependency" />
      ) : null}
      {actionErr ? <StatusLine kind="error">{actionErr}</StatusLine> : null}
      {ok && !actionErr ? (
        <p role="status" className="m-0 text-sm text-success">
          {ok}
        </p>
      ) : null}
      {!createBlocked && showForm ? (
        <Card>
          <CardHeader icon="plus" title={t("functions.cron.create")} />
          <div className="flex flex-col gap-3 p-4">
            <label className={labelClass}>
              {t("functions.cron.spec")}
              <input className={fieldClass + " mt-1"} value={cronSpec} onChange={(e) => setCronSpec(e.target.value)} autoComplete="off" />
            </label>
            <label className={labelClass}>
              {t("functions.cron.session")}
              <select className={selectClass + " mt-1 w-full"} value={cronSession} onChange={(e) => setCronSession(e.target.value)}>
                <option value="">{t("functions.cron.pickSession")}</option>
                {sessions.map((s) => (
                  <option key={s.id} value={s.id}>
                    {s.label || s.id}
                  </option>
                ))}
              </select>
            </label>
            <label className={labelClass}>
              {t("functions.cron.message")}
              <input className={fieldClass + " mt-1"} value={cronMessage} onChange={(e) => setCronMessage(e.target.value)} autoComplete="off" />
            </label>
            <div className="flex gap-2">
              <Button variant="default" onClick={() => void createCron()}>
                {t("functions.cron.create")}
              </Button>
              <Button variant="outline" onClick={() => setShowForm(false)}>
                {t("common.cancel")}
              </Button>
            </div>
          </div>
        </Card>
      ) : null}
      {jobsState.showEmpty ? <EmptyState icon="bolt" title={t("functions.cron.empty")} data-page-state="empty" /> : null}
      {filteredEmpty ? <EmptyState icon="search" title={t("functions.cron.filterEmpty")} data-page-state="filtered_empty" /> : null}
      {jobsState.showItems || jobsState.showEmpty || filteredEmpty ? (
        <p className="m-0 text-xs text-muted-foreground">{metaN == null ? "—" : t("functions.cron.meta", { n: metaN })}</p>
      ) : null}
      {jobsState.showItems ? (
        <DataTable minWidth="900px">
          <THead>
            <Th>{t("functions.cron.col.spec")}</Th>
            <Th>{t("functions.cron.col.session")}</Th>
            <Th>{t("functions.cron.col.message")}</Th>
            <Th>{t("functions.cron.col.last")}</Th>
            <Th>{t("functions.cron.col.error")}</Th>
            <Th>{t("functions.cron.col.enabled")}</Th>
            <Th className="text-right" />
          </THead>
          <tbody>
            {visible.map((job) => (
              <Tr key={job.id}>
                <Td className="font-semibold">{job.spec}</Td>
                <Td className="text-muted-foreground">{job.session_id}</Td>
                <Td className="text-muted-foreground">{job.message}</Td>
                <Td className="text-muted-foreground">{job.last_run || "—"}</Td>
                <Td className="text-xs text-destructive">{job.last_error ? redactPublicText(job.last_error) : "—"}</Td>
                <Td onClick={(e) => e.stopPropagation()}>
                  <Button variant={job.enabled !== false ? "default" : "outline"} size="sm" disabled={blocked} onClick={() => void toggleCron(job)}>
                    {job.enabled !== false ? t("common.enabled") : t("common.disabled")}
                  </Button>
                </Td>
                <Td className="text-right" onClick={(e) => e.stopPropagation()}>
                  <Button variant="ghost" size="sm" className="text-destructive" disabled={blocked} onClick={() => void deleteCron(job)}>
                    {t("common.delete")}
                  </Button>
                </Td>
              </Tr>
            ))}
          </tbody>
        </DataTable>
      ) : null}
    </PageChrome>
  );
}
