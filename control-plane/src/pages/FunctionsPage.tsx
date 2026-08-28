import { useEffect, useState } from "react";
import { api, type Agent, type Connector, type Session } from "../api/client";
import { cronApi, type CronJob } from "../api/cron";
import { skillsApi, type SkillInfo } from "../api/skills";
import { toolsApi, type AgentTool } from "../api/tools";
import { useI18n } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { SectionHeader } from "../ui/SectionHeader";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

export function FunctionsPage() {
  const { t } = useI18n();
  const [agents, setAgents] = useState<Agent[]>([]);
  const [connectors, setConnectors] = useState<Connector[]>([]);
  const [agentId, setAgentId] = useState("");
  const [tools, setTools] = useState<AgentTool[]>([]);
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(true);
  const [toolsLoading, setToolsLoading] = useState(false);
  const [notFound, setNotFound] = useState(false);
  const [connName, setConnName] = useState("");
  const [endpoint, setEndpoint] = useState("");
  const [token, setToken] = useState("");
  const [skills, setSkills] = useState<SkillInfo[]>([]);
  const [skillsLoading, setSkillsLoading] = useState(true);
  const [skillsErr, setSkillsErr] = useState("");
  const [sessions, setSessions] = useState<Session[]>([]);
  const [sessionsLoaded, setSessionsLoaded] = useState(false);
  const [cronJobs, setCronJobs] = useState<CronJob[]>([]);
  const [cronLoading, setCronLoading] = useState(true);
  const [cronErr, setCronErr] = useState("");
  const [cronSpec, setCronSpec] = useState("every:1h");
  const [cronSession, setCronSession] = useState("");
  const [cronMessage, setCronMessage] = useState("");

  const selected = connectors.find((c) => c.name === connName);

  async function loadAgents() {
    try {
      const [a, c] = await Promise.all([api.listAgents(), api.listConnectors()]);
      setAgents(a.agents ?? []);
      setConnectors(c.connectors ?? []);
      setErr("");
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setLoading(false);
    }
  }

  async function loadTools(id: string) {
    if (!id) {
      setTools([]);
      setNotFound(false);
      setToolsLoading(false);
      return;
    }
    setToolsLoading(true);
    try {
      const j = await toolsApi.list(id);
      setTools(j.tools ?? []);
      setNotFound(false);
      setErr("");
    } catch (e) {
      const msg = formatPublicError(e);
      setTools([]);
      setNotFound(msg.includes("404"));
      setErr(msg);
    } finally {
      setToolsLoading(false);
    }
  }

  async function loadSkills() {
    setSkillsLoading(true);
    try {
      const j = await skillsApi.list();
      setSkills(j.skills ?? []);
      setSkillsErr("");
    } catch (e) {
      setSkills([]);
      setSkillsErr(formatPublicError(e));
    } finally {
      setSkillsLoading(false);
    }
  }

  async function loadCron() {
    setCronLoading(true);
    const [jobsRes, sessRes] = await Promise.allSettled([cronApi.list(), api.listSessions()]);
    if (jobsRes.status === "fulfilled") {
      setCronJobs(jobsRes.value.jobs ?? []);
    } else {
      setCronJobs([]);
    }
    if (sessRes.status === "fulfilled") {
      setSessions(sessRes.value.sessions ?? []);
      setSessionsLoaded(true);
    } else {
      setSessionsLoaded(false);
    }
    const fail = jobsRes.status === "rejected" ? jobsRes.reason : sessRes.status === "rejected" ? sessRes.reason : null;
    setCronErr(fail ? formatPublicError(fail) : "");
    setCronLoading(false);
  }

  useEffect(() => {
    void loadAgents();
    void loadSkills();
    void loadCron();
  }, []);

  useEffect(() => {
    void loadTools(agentId);
  }, [agentId]);

  useEffect(() => {
    if (!selected) {
      setEndpoint("");
      setToken("");
      return;
    }
    setEndpoint(selected.endpoint ?? "");
    setToken("");
  }, [connName, selected?.endpoint]);

  async function toggle(tool: AgentTool) {
    if (!agentId) return;
    try {
      await toolsApi.setEnabled(agentId, tool.name, !tool.enabled);
      await loadTools(agentId);
      await loadAgents();
    } catch (e) {
      setErr(formatPublicError(e));
    }
  }

  async function saveConnector() {
    setErr("");
    if (!connName.trim()) {
      setErr(t("functions.needConnector"));
      return;
    }
    try {
      const body: { endpoint?: string; token?: string } = { endpoint };
      if (token.trim()) body.token = token.trim();
      await toolsApi.patchConnector(connName, body);
      setToken("");
      await loadAgents();
    } catch (e) {
      setErr(formatPublicError(e));
    }
  }

  async function createCron() {
    setCronErr("");
    if (!cronSpec.trim()) {
      setCronErr(t("functions.cron.needSpec"));
      return;
    }
    if (sessionsLoaded && sessions.length === 0) {
      setCronErr(t("functions.cron.noSessions"));
      return;
    }
    if (!cronSession.trim()) {
      setCronErr(t("functions.cron.needSession"));
      return;
    }
    if (!cronMessage.trim()) {
      setCronErr(t("functions.cron.needMessage"));
      return;
    }
    try {
      await cronApi.create({ spec: cronSpec.trim(), session_id: cronSession.trim(), message: cronMessage.trim() });
      setCronMessage("");
      setCronErr("");
      await loadCron();
    } catch (e) {
      setCronErr(formatPublicError(e));
    }
  }

  async function deleteCron(id: string) {
    if (!window.confirm(t("functions.cron.confirmDelete"))) return;
    try {
      await cronApi.remove(id);
      setCronErr("");
      await loadCron();
    } catch (e) {
      setCronErr(formatPublicError(e));
    }
  }

  return (
    <div style={{ padding: "14px 22px 40px", display: "flex", flexDirection: "column", gap: 14 }}>
      <SectionHeader
        icon="build"
        title={t("functions.title")}
        description={t("functions.desc")}
        actions={
          <Button
            icon="refresh"
            iconGesture
            onClick={() => {
              void loadAgents();
              void loadTools(agentId);
              void loadSkills();
              void loadCron();
            }}
          >
            {t("common.refresh")}
          </Button>
        }
      />
      {err ? <StatusLine kind="error">{err}</StatusLine> : null}
      <Card style={{ padding: 14, display: "flex", gap: 8, flexWrap: "wrap", alignItems: "center" }}>
        <select className="z-field" value={agentId} onChange={(e) => setAgentId(e.target.value)}>
          <option value="">{t("functions.pickAgent")}</option>
          {agents.map((a) => (
            <option key={a.id} value={a.id}>
              {a.display_name || a.agent_key}
            </option>
          ))}
        </select>
      </Card>
      <Card>
        <CardHeader icon="build" title={t("functions.tools")} meta={t("functions.meta", { n: tools.length })} />
        <p style={{ margin: 0, padding: "8px 16px 0", fontSize: 12, color: "var(--text-3)" }}>{t("functions.workspace.note")}</p>
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
          <span style={{ flex: 1.6 }}>{t("functions.col.name")}</span>
          <span style={{ flex: 1.2 }}>{t("functions.col.connector")}</span>
          <span style={{ flex: 1 }}>{t("functions.col.approval")}</span>
          <span style={{ flex: 0.8, textAlign: "right" }}>{t("functions.col.on")}</span>
        </div>
        {tools.map((tool) => (
          <div
            key={`${tool.connector}:${tool.name}`}
            style={{ display: "flex", alignItems: "center", padding: "11px 16px", fontSize: 12.5, borderBottom: "1px solid var(--border-soft)" }}
          >
            <span style={{ flex: 1.6, fontWeight: 600 }}>{tool.name}</span>
            <span style={{ flex: 1.2, color: "var(--text-2)" }}>{tool.connector}</span>
            <span style={{ flex: 1 }}>
              <Badge tone={tool.requires_approval ? "warning" : "neutral"}>
                {tool.requires_approval ? t("functions.approvalYes") : t("functions.approvalNo")}
              </Badge>
            </span>
            <span style={{ flex: 0.8, textAlign: "right" }}>
              <Button variant={tool.enabled ? "primary" : "ghost"} onClick={() => void toggle(tool)}>
                {tool.enabled ? t("common.enabled") : t("common.disabled")}
              </Button>
            </span>
          </div>
        ))}
        {loading || toolsLoading ? <StatusLine kind="loading" /> : null}
        {!loading && !toolsLoading && !agentId ? <EmptyState>{t("functions.emptyAgent")}</EmptyState> : null}
        {!loading && !toolsLoading && agentId && notFound ? <EmptyState>{t("functions.notFound")}</EmptyState> : null}
        {!loading && !toolsLoading && agentId && !notFound && tools.length === 0 ? <EmptyState>{t("functions.empty")}</EmptyState> : null}
        </TableScroll>
      </Card>
      <Card>
        <CardHeader icon="hook" title={t("functions.connectors")} />
        <div style={{ padding: 14, display: "flex", flexDirection: "column", gap: 10 }}>
          <div style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "center" }}>
            <select className="z-field" value={connName} onChange={(e) => setConnName(e.target.value)}>
              <option value="">{t("functions.pickConnector")}</option>
              {connectors.map((c) => (
                <option key={c.name} value={c.name}>
                  {c.name}
                </option>
              ))}
            </select>
            {selected ? (
              <Badge tone={selected.token_set ? "positive" : "neutral"}>
                {t("functions.tokenSet")}: {selected.token_set ? t("common.yes") : t("common.no")}
              </Badge>
            ) : null}
          </div>
          {connName ? (
            <>
              <label style={{ fontSize: 12, color: "var(--text-2)" }}>
                {t("functions.endpoint")}
                <input
                  className="z-field"
                  style={{ display: "block", width: "100%", marginTop: 4 }}
                  value={endpoint}
                  onChange={(e) => setEndpoint(e.target.value)}
                />
              </label>
              <label style={{ fontSize: 12, color: "var(--text-2)" }}>
                {t("functions.token")}
                <input
                  className="z-field"
                  type="password"
                  autoComplete="off"
                  style={{ display: "block", width: "100%", marginTop: 4 }}
                  value={token}
                  onChange={(e) => setToken(e.target.value)}
                />
              </label>
              <p style={{ margin: 0, fontSize: 12, color: "var(--text-3)" }}>{t("functions.tokenHint")}</p>
            </>
          ) : null}
          <div>
            <Button variant="primary" onClick={() => void saveConnector()}>
              {t("functions.saveConnector")}
            </Button>
          </div>
          {loading ? <StatusLine kind="loading" /> : connectors.length === 0 ? <EmptyState>{t("functions.emptyConnectors")}</EmptyState> : null}
        </div>
      </Card>
      <Card>
        <CardHeader icon="doc" title={t("functions.skills")} meta={t("functions.skills.meta", { n: skills.length })} />
        {skillsErr ? <StatusLine kind="error">{skillsErr}</StatusLine> : null}
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
          <span style={{ flex: 1.4 }}>{t("functions.skills.col.name")}</span>
          <span style={{ flex: 2 }}>{t("functions.skills.col.path")}</span>
        </div>
        {skills.map((s) => (
          <div
            key={s.name}
            style={{ display: "flex", alignItems: "center", padding: "11px 16px", fontSize: 12.5, borderBottom: "1px solid var(--border-soft)" }}
          >
            <span style={{ flex: 1.4, fontWeight: 600 }}>{s.name}</span>
            <span style={{ flex: 2, color: "var(--text-2)" }}>{s.path}</span>
          </div>
        ))}
        {skillsLoading ? <StatusLine kind="loading" /> : null}
        {!skillsLoading && !skillsErr && skills.length === 0 ? <EmptyState>{t("functions.skills.empty")}</EmptyState> : null}
        </TableScroll>
      </Card>
      <Card>
        <CardHeader icon="timer" title={t("functions.cron")} meta={t("functions.cron.meta", { n: cronJobs.length })} />
        {cronErr ? <StatusLine kind="error">{cronErr}</StatusLine> : null}
        <div style={{ padding: 14, display: "flex", flexDirection: "column", gap: 10 }}>
          {!cronLoading && sessionsLoaded && sessions.length === 0 ? <EmptyState>{t("functions.cron.noSessions")}</EmptyState> : null}
          <div style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "flex-end" }}>
            <label style={{ fontSize: 12, color: "var(--text-2)", flex: "1 1 160px" }}>
              {t("functions.cron.spec")}
              <input
                className="z-field"
                style={{ display: "block", width: "100%", marginTop: 4 }}
                value={cronSpec}
                onChange={(e) => setCronSpec(e.target.value)}
              />
            </label>
            <label style={{ fontSize: 12, color: "var(--text-2)", flex: "1 1 160px" }}>
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
            <label style={{ fontSize: 12, color: "var(--text-2)", flex: "2 1 220px" }}>
              {t("functions.cron.message")}
              <input
                className="z-field"
                style={{ display: "block", width: "100%", marginTop: 4 }}
                value={cronMessage}
                onChange={(e) => setCronMessage(e.target.value)}
              />
            </label>
            <Button variant="primary" onClick={() => void createCron()}>
              {t("functions.cron.create")}
            </Button>
          </div>
        </div>
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
          <span style={{ flex: 1.2 }}>{t("functions.cron.col.spec")}</span>
          <span style={{ flex: 1.2 }}>{t("functions.cron.col.session")}</span>
          <span style={{ flex: 1.6 }}>{t("functions.cron.col.message")}</span>
          <span style={{ flex: 1 }}>{t("functions.cron.col.last")}</span>
          <span style={{ flex: 0.7 }}>{t("functions.cron.col.enabled")}</span>
          <span style={{ flex: 0.6 }} />
        </div>
        {cronJobs.map((job) => (
          <div
            key={job.id}
            style={{ display: "flex", alignItems: "center", padding: "11px 16px", fontSize: 12.5, borderBottom: "1px solid var(--border-soft)", gap: 8 }}
          >
            <span style={{ flex: 1.2, fontWeight: 600 }}>{job.spec}</span>
            <span style={{ flex: 1.2, color: "var(--text-2)" }}>{job.session_id}</span>
            <span style={{ flex: 1.6, color: "var(--text-2)" }}>{job.message}</span>
            <span style={{ flex: 1, color: "var(--text-3)" }}>{job.last_run || "—"}</span>
            <span style={{ flex: 0.7 }}>
              {typeof job.enabled === "boolean" ? (
                <Badge tone={job.enabled ? "positive" : "neutral"}>{job.enabled ? t("common.enabled") : t("common.disabled")}</Badge>
              ) : (
                "—"
              )}
            </span>
            <span style={{ flex: 0.6, textAlign: "right" }}>
              <Button variant="ghost" onClick={() => void deleteCron(job.id)}>
                {t("common.delete")}
              </Button>
            </span>
          </div>
        ))}
        {cronLoading ? <StatusLine kind="loading" /> : null}
        {!cronLoading && !cronErr && cronJobs.length === 0 ? <EmptyState>{t("functions.cron.empty")}</EmptyState> : null}
        </TableScroll>
      </Card>
    </div>
  );
}
