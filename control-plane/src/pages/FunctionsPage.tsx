import { useEffect, useState } from "react";
import { api, type Agent, type Connector } from "../api/client";
import { toolsApi, type AgentTool } from "../api/tools";
import { useI18n } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader } from "../ui/Card";
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

  useEffect(() => {
    void loadAgents();
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
    if (!connName) return;
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
              <div>
                <Button variant="primary" onClick={() => void saveConnector()}>
                  {t("functions.saveConnector")}
                </Button>
              </div>
            </>
          ) : null}
          {loading ? <StatusLine kind="loading" /> : connectors.length === 0 ? <EmptyState>{t("functions.emptyConnectors")}</EmptyState> : null}
        </div>
      </Card>
    </div>
  );
}
