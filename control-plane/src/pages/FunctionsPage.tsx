import { useEffect, useRef, useState } from "react";
import { api, type Agent, type Connector, type Session } from "../api/client";
import { cronApi, type CronJob } from "../api/cron";
import {
  connectorWriteBody,
  formatConnectorTest,
  isConnectorEnvOwned,
  type ConnectorTestView,
} from "../api/function-ops";
import { skillsApi, type SkillInfo } from "../api/skills";
import { toolsApi, type AgentTool } from "../api/tools";
import { useI18n } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { SectionHeader } from "../ui/SectionHeader";
import { StatusLine, formatPublicError, redactPublicText } from "../ui/StatusLine";

const emptyConnForm = { name: "", transport: "mcp-http", endpoint: "", token: "", credential_ref: "", enabled: true };

type FunctionFocus = "skills" | "tools" | "mcp" | "cron";

export function FunctionsPage({ focus = "tools" }: { focus?: FunctionFocus }) {
  const { t } = useI18n();
  const [agents, setAgents] = useState<Agent[]>([]);
  const [connectors, setConnectors] = useState<Connector[]>([]);
  const [agentId, setAgentId] = useState("");
  const [tools, setTools] = useState<AgentTool[]>([]);
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(true);
  const [toolsLoading, setToolsLoading] = useState(false);
  const [notFound, setNotFound] = useState(false);
  const [connErr, setConnErr] = useState("");
  const [connForm, setConnForm] = useState(emptyConnForm);
  const [connName, setConnName] = useState("");
  const [endpoint, setEndpoint] = useState("");
  const [token, setToken] = useState("");
  const [testing, setTesting] = useState(false);
  const [testView, setTestView] = useState<ConnectorTestView | null>(null);
  const [skills, setSkills] = useState<SkillInfo[]>([]);
  const [skillsLoading, setSkillsLoading] = useState(true);
  const [skillsErr, setSkillsErr] = useState("");
  const [skillQuery, setSkillQuery] = useState("");
  const [skillName, setSkillName] = useState("");
  const [skillBody, setSkillBody] = useState("");
  const skillReq = useRef(0);
  const [sessions, setSessions] = useState<Session[]>([]);
  const [sessionsLoaded, setSessionsLoaded] = useState(false);
  const [cronJobs, setCronJobs] = useState<CronJob[]>([]);
  const [cronLoading, setCronLoading] = useState(true);
  const [cronErr, setCronErr] = useState("");
  const [cronSpec, setCronSpec] = useState("every:1h");
  const [cronSession, setCronSession] = useState("");
  const [cronMessage, setCronMessage] = useState("");

  const selected = connectors.find((c) => c.name === connName);
  const envLocked = selected ? isConnectorEnvOwned(selected) : false;

  useEffect(() => {
    document.getElementById(`functions-${focus}`)?.scrollIntoView({ block: "start" });
  }, [focus]);

  async function loadAgents() {
    try {
      const [a, c] = await Promise.all([api.listAgents(), api.listConnectors()]);
      setAgents(a.agents ?? []);
      setConnectors(c.connectors ?? []);
      setErr("");
      setConnErr("");
    } catch (e) {
      setErr(formatPublicError(e));
      setConnErr(formatPublicError(e));
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

  async function loadSkills(q = skillQuery) {
    const req = ++skillReq.current;
    setSkillsLoading(true);
    try {
      const trimmed = q.trim();
      const j = trimmed ? await skillsApi.search(trimmed) : await skillsApi.list();
      if (req !== skillReq.current) return;
      setSkills(j.skills ?? []);
      setSkillsErr("");
    } catch (e) {
      if (req !== skillReq.current) return;
      setSkills([]);
      setSkillsErr(formatPublicError(e));
    } finally {
      if (req === skillReq.current) setSkillsLoading(false);
    }
  }

  async function createSkill() {
    const name = skillName.trim();
    if (!/^[a-z0-9_-]{1,64}$/.test(name)) {
      setSkillsErr(t("functions.skills.needName"));
      return;
    }
    if (!skillBody.trim()) {
      setSkillsErr(t("functions.skills.needBody"));
      return;
    }
    try {
      await skillsApi.create({ name, body: skillBody });
      setSkillName("");
      setSkillBody("");
      setSkillsErr("");
      await loadSkills();
    } catch (e) {
      setSkillsErr(formatPublicError(e));
    }
  }

  async function archiveSkill(name: string) {
    if (!window.confirm(t("functions.skills.confirmDelete"))) return;
    try {
      await skillsApi.remove(name);
      setSkillsErr("");
      await loadSkills();
    } catch (e) {
      setSkillsErr(formatPublicError(e));
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
      setTestView(null);
      return;
    }
    setEndpoint(selected.endpoint ?? "");
    setToken("");
    setTestView(null);
  }, [connName, selected?.endpoint]);

  async function toggle(tool: AgentTool) {
    if (!agentId) return;
    try {
      await toolsApi.setEnabled(agentId, tool.name, !tool.enabled);
      await loadTools(agentId);
    } catch (e) {
      setErr(formatPublicError(e));
    }
  }

  async function addConnector() {
    setConnErr("");
    setTestView(null);
    const body = connectorWriteBody(connForm);
    if (!body.name) {
      setConnErr(t("functions.mcp.needName"));
      return;
    }
    if (!body.endpoint) {
      setConnErr(t("functions.mcp.needEndpoint"));
      return;
    }
    try {
      const created = await toolsApi.createConnector(body);
      setConnForm(emptyConnForm);
      setConnName(created.name);
      setToken("");
      await loadAgents();
    } catch (e) {
      setConnErr(formatPublicError(e));
    }
  }

  async function saveConnector() {
    setConnErr("");
    if (!connName.trim()) {
      setConnErr(t("functions.needConnector"));
      return;
    }
    if (envLocked) {
      setConnErr(t("functions.mcp.envLocked"));
      return;
    }
    try {
      const body = connectorWriteBody({ endpoint, token, enabled: selected?.enabled });
      await toolsApi.patchConnector(connName, body);
      setToken("");
      await loadAgents();
    } catch (e) {
      setConnErr(formatPublicError(e));
    }
  }

  async function toggleConnector(c: Connector) {
    setConnErr("");
    try {
      await toolsApi.patchConnector(c.name, { enabled: !c.enabled });
      await loadAgents();
    } catch (e) {
      setConnErr(formatPublicError(e));
    }
  }

  async function testConnector(name: string) {
    if (!name.trim()) {
      setConnErr(t("functions.needConnector"));
      return;
    }
    setTesting(true);
    setConnErr("");
    try {
      const result = await toolsApi.testConnector(name);
      setTestView(formatConnectorTest(result));
    } catch (e) {
      setTestView(null);
      setConnErr(formatPublicError(e));
    } finally {
      setTesting(false);
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

  async function toggleCron(job: CronJob) {
    try {
      await cronApi.setEnabled(job.id, !(job.enabled !== false));
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
      <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("functions.mcp.noSecrets")}</p>
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
      <Card id="functions-tools">
        <CardHeader icon="build" title={t("functions.tools")} meta={t("functions.meta", { n: tools.length })} />
        {err ? <StatusLine kind="error">{err}</StatusLine> : null}
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
          <span style={{ flex: 1.5 }}>{t("functions.col.name")}</span>
          <span style={{ flex: 1.1 }}>{t("functions.col.connector")}</span>
          <span style={{ flex: 0.9 }}>{t("functions.col.approval")}</span>
          <span style={{ flex: 1 }}>{t("functions.col.configured")}</span>
          <span style={{ flex: 0.9 }}>{t("functions.col.granted")}</span>
          <span style={{ flex: 0.8, textAlign: "right" }}>{t("functions.col.on")}</span>
        </div>
        {tools.map((tool) => (
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
      <Card id="functions-mcp">
        <CardHeader icon="hook" title={t("functions.connectors")} meta={t("functions.mcp.meta", { n: connectors.length })} />
        {connErr ? <StatusLine kind="error">{connErr}</StatusLine> : null}
        <div style={{ padding: 14, display: "flex", flexDirection: "column", gap: 10 }}>
          <div style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "flex-end" }}>
            <label style={{ fontSize: 12, color: "var(--text-2)", flex: "1 1 140px" }}>
              {t("functions.mcp.name")}
              <input className="z-field" style={{ display: "block", width: "100%", marginTop: 4 }} value={connForm.name} onChange={(e) => setConnForm((f) => ({ ...f, name: e.target.value }))} />
            </label>
            <label style={{ fontSize: 12, color: "var(--text-2)", flex: "0 1 160px" }}>
              {t("functions.mcp.transport")}
              <select className="z-field" style={{ display: "block", width: "100%", marginTop: 4 }} value={connForm.transport} onChange={(e) => setConnForm((f) => ({ ...f, transport: e.target.value }))}>
                <option value="http">{t("functions.mcp.http")}</option>
                <option value="mcp-http">{t("functions.mcp.sse")}</option>
                <option value="mcp-stdio">{t("functions.mcp.stdio")}</option>
              </select>
            </label>
            <label style={{ fontSize: 12, color: "var(--text-2)", flex: "2 1 220px" }}>
              {t("functions.endpoint")}
              <input className="z-field" style={{ display: "block", width: "100%", marginTop: 4 }} value={connForm.endpoint} onChange={(e) => setConnForm((f) => ({ ...f, endpoint: e.target.value }))} />
            </label>
            <label style={{ fontSize: 12, color: "var(--text-2)", flex: "1 1 160px" }}>
              {t("functions.token")}
              <input className="z-field" type="password" autoComplete="off" style={{ display: "block", width: "100%", marginTop: 4 }} value={connForm.token} onChange={(e) => setConnForm((f) => ({ ...f, token: e.target.value }))} />
            </label>
            <label style={{ fontSize: 12, color: "var(--text-2)", flex: "1 1 160px" }}>
              {t("functions.mcp.envName")}
              <input className="z-field" style={{ display: "block", width: "100%", marginTop: 4 }} value={connForm.credential_ref} onChange={(e) => setConnForm((f) => ({ ...f, credential_ref: e.target.value }))} />
            </label>
            <Button variant="primary" onClick={() => void addConnector()}>
              {t("functions.mcp.add")}
            </Button>
          </div>
          <p style={{ margin: 0, fontSize: 12, color: "var(--text-3)" }}>{t("functions.mcp.envHint")}</p>
        </div>
        <TableScroll>
        <div style={{ display: "flex", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)", gap: 8 }}>
          <span style={{ flex: 1.2 }}>{t("functions.col.name")}</span>
          <span style={{ flex: 1 }}>{t("functions.mcp.transport")}</span>
          <span style={{ flex: 1.6 }}>{t("functions.endpoint")}</span>
          <span style={{ flex: 1 }}>{t("functions.tokenSet")}</span>
          <span style={{ flex: 0.9 }}>{t("functions.col.on")}</span>
          <span style={{ flex: 0.8 }} />
        </div>
        {connectors.map((c) => (
          <div
            key={c.name}
            role="button"
            tabIndex={0}
            onClick={() => setConnName(c.name)}
            onKeyDown={(e) => {
              if (e.key === "Enter" || e.key === " ") setConnName(c.name);
            }}
            style={{
              display: "flex",
              width: "100%",
              textAlign: "left",
              alignItems: "center",
              padding: "11px 16px",
              fontSize: 12.5,
              borderBottom: "1px solid var(--border-soft)",
              background: connName === c.name ? "var(--bg-2)" : "transparent",
              cursor: "pointer",
              gap: 8,
            }}
          >
            <span style={{ flex: 1.2, fontWeight: 600 }}>{c.name}</span>
            <span style={{ flex: 1, color: "var(--text-2)" }}>{c.transport}</span>
            <span style={{ flex: 1.6, color: "var(--text-3)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{c.endpoint}</span>
            <span style={{ flex: 1, display: "flex", gap: 6, flexWrap: "wrap" }}>
              <Badge tone={c.token_set ? "positive" : "neutral"}>{c.token_set ? t("common.yes") : t("common.no")}</Badge>
              {isConnectorEnvOwned(c) ? <Badge tone="warning">{t("functions.mcp.envOwned")}</Badge> : <Badge tone="neutral">{t("functions.mcp.source.sqlite")}</Badge>}
              {!c.enabled ? <Badge tone="neutral">{t("functions.mcp.disabled")}</Badge> : null}
            </span>
            <span style={{ flex: 0.9 }}>
              <Badge tone={c.enabled ? "positive" : "neutral"}>{c.enabled ? t("common.enabled") : t("common.disabled")}</Badge>
            </span>
            <span style={{ flex: 0.8, textAlign: "right" }} onClick={(e) => e.stopPropagation()}>
              <Button variant="ghost" onClick={() => void toggleConnector(c)}>
                {c.enabled ? t("common.disabled") : t("functions.cron.enable")}
              </Button>
            </span>
          </div>
        ))}
        {loading ? <StatusLine kind="loading" /> : connectors.length === 0 ? <EmptyState>{t("functions.emptyConnectors")}</EmptyState> : null}
        </TableScroll>
        {connName ? (
          <div style={{ padding: 14, display: "flex", flexDirection: "column", gap: 10 }}>
            {envLocked ? <p style={{ margin: 0, fontSize: 12, color: "var(--text-3)" }}>{t("functions.mcp.envLocked")}</p> : null}
            <label style={{ fontSize: 12, color: "var(--text-2)" }}>
              {t("functions.endpoint")}
              <input className="z-field" style={{ display: "block", width: "100%", marginTop: 4 }} value={endpoint} onChange={(e) => setEndpoint(e.target.value)} disabled={envLocked} />
            </label>
            <label style={{ fontSize: 12, color: "var(--text-2)" }}>
              {t("functions.token")}
              <input className="z-field" type="password" autoComplete="off" style={{ display: "block", width: "100%", marginTop: 4 }} value={token} onChange={(e) => setToken(e.target.value)} disabled={envLocked} />
            </label>
            <p style={{ margin: 0, fontSize: 12, color: "var(--text-3)" }}>{t("functions.tokenHint")}</p>
            <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
              <Button variant="primary" onClick={() => void saveConnector()} disabled={envLocked}>
                {t("functions.saveConnector")}
              </Button>
              <Button onClick={() => void testConnector(connName)} disabled={testing}>
                {testing ? t("functions.mcp.testing") : t("functions.mcp.test")}
              </Button>
            </div>
            {testView ? (
              <p style={{ margin: 0, fontSize: 12.5, color: testView.ok ? "var(--green)" : "var(--red)" }}>
                {testView.health} · {testView.latency_ms}ms{testView.error ? ` · ${redactPublicText(testView.error)}` : ""}
              </p>
            ) : null}
          </div>
        ) : null}
      </Card>
      <Card id="functions-skills">
        <CardHeader icon="doc" title={t("functions.skills")} meta={t("functions.skills.meta", { n: skills.length })} />
        {skillsErr ? <StatusLine kind="error">{skillsErr}</StatusLine> : null}
        <div style={{ padding: 14, display: "flex", flexDirection: "column", gap: 10 }}>
          <label style={{ fontSize: 12, color: "var(--text-2)" }}>
            {t("functions.skills.search")}
            <input
              className="z-field"
              style={{ display: "block", width: "100%", marginTop: 4 }}
              value={skillQuery}
              onChange={(e) => {
                const v = e.target.value;
                setSkillQuery(v);
                void loadSkills(v);
              }}
            />
          </label>
          <div style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "flex-end" }}>
            <label style={{ fontSize: 12, color: "var(--text-2)", flex: "1 1 160px" }}>
              {t("functions.skills.name")}
              <input className="z-field" style={{ display: "block", width: "100%", marginTop: 4 }} value={skillName} onChange={(e) => setSkillName(e.target.value)} />
            </label>
            <label style={{ fontSize: 12, color: "var(--text-2)", flex: "2 1 240px" }}>
              {t("functions.skills.body")}
              <textarea className="z-field" style={{ display: "block", width: "100%", marginTop: 4, minHeight: 64, fontFamily: "var(--font-mono, monospace)" }} value={skillBody} onChange={(e) => setSkillBody(e.target.value)} />
            </label>
            <Button variant="primary" onClick={() => void createSkill()}>
              {t("functions.skills.create")}
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
          <span style={{ flex: 1.2 }}>{t("functions.skills.col.name")}</span>
          <span style={{ flex: 1.6 }}>{skillQuery.trim() ? t("functions.skills.col.snippet") : t("functions.skills.col.path")}</span>
          <span style={{ flex: 0.6 }} />
        </div>
        {skills.map((s) => (
          <div
            key={s.name}
            style={{ display: "flex", alignItems: "center", padding: "11px 16px", fontSize: 12.5, borderBottom: "1px solid var(--border-soft)", gap: 8 }}
          >
            <span style={{ flex: 1.2, fontWeight: 600 }}>{s.name}</span>
            <span style={{ flex: 1.6, color: "var(--text-2)" }}>{skillQuery.trim() ? s.snippet ?? "" : s.path ?? ""}</span>
            <span style={{ flex: 0.6, textAlign: "right" }}>
              <Button variant="ghost" onClick={() => void archiveSkill(s.name)}>
                {t("functions.skills.archive")}
              </Button>
            </span>
          </div>
        ))}
        {skillsLoading ? <StatusLine kind="loading" /> : null}
        {!skillsLoading && !skillsErr && skills.length === 0 ? <EmptyState>{t("functions.skills.empty")}</EmptyState> : null}
        </TableScroll>
      </Card>
      <Card id="functions-cron">
        <CardHeader icon="timer" title={t("functions.cron")} meta={t("functions.cron.meta", { n: cronJobs.length })} />
        {cronErr ? <StatusLine kind="error">{cronErr}</StatusLine> : null}
        <div style={{ padding: 14, display: "flex", flexDirection: "column", gap: 10 }}>
          {!cronLoading && sessionsLoaded && sessions.length === 0 ? <EmptyState>{t("functions.cron.noSessions")}</EmptyState> : null}
          <div style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "flex-end" }}>
            <label style={{ fontSize: 12, color: "var(--text-2)", flex: "1 1 160px" }}>
              {t("functions.cron.spec")}
              <input className="z-field" style={{ display: "block", width: "100%", marginTop: 4 }} value={cronSpec} onChange={(e) => setCronSpec(e.target.value)} />
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
              <input className="z-field" style={{ display: "block", width: "100%", marginTop: 4 }} value={cronMessage} onChange={(e) => setCronMessage(e.target.value)} />
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
          <span style={{ flex: 1.1 }}>{t("functions.cron.col.spec")}</span>
          <span style={{ flex: 1.1 }}>{t("functions.cron.col.session")}</span>
          <span style={{ flex: 1.3 }}>{t("functions.cron.col.message")}</span>
          <span style={{ flex: 0.9 }}>{t("functions.cron.col.last")}</span>
          <span style={{ flex: 1.1 }}>{t("functions.cron.col.error")}</span>
          <span style={{ flex: 0.7 }}>{t("functions.cron.col.enabled")}</span>
          <span style={{ flex: 0.6 }} />
        </div>
        {cronJobs.map((job) => (
          <div
            key={job.id}
            style={{ display: "flex", alignItems: "center", padding: "11px 16px", fontSize: 12.5, borderBottom: "1px solid var(--border-soft)", gap: 8 }}
          >
            <span style={{ flex: 1.1, fontWeight: 600 }}>{job.spec}</span>
            <span style={{ flex: 1.1, color: "var(--text-2)" }}>{job.session_id}</span>
            <span style={{ flex: 1.3, color: "var(--text-2)" }}>{job.message}</span>
            <span style={{ flex: 0.9, color: "var(--text-3)" }}>{job.last_run || "—"}</span>
            <span style={{ flex: 1.1, color: "var(--red)", fontSize: 11 }}>{job.last_error ? redactPublicText(job.last_error) : "—"}</span>
            <span style={{ flex: 0.7 }}>
              <Button variant={job.enabled !== false ? "primary" : "ghost"} onClick={() => void toggleCron(job)}>
                {job.enabled !== false ? t("common.enabled") : t("common.disabled")}
              </Button>
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
