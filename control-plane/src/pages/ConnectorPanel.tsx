import { useEffect, useMemo, useState } from "react";
import { api, type Agent } from "../api/client";
import type { ConnectorInfo } from "../api/function-ops";
import { resolveSettled } from "../api/channel-ops";
import {
  connectorFormError,
  connectorRowLeaksSecret,
  connectorTestReady,
  filterByQuery,
  publicConnector,
  type ConnectorFormError,
} from "../api/capabilities-ops";
import {
  connectorWriteBody,
  formatConnectorTest,
  isConnectorEnvOwned,
  type ConnectorTestView,
} from "../api/function-ops";
import { classifyPageState, formatStaleAt, inventoryBlocksMutation, isFilteredEmpty, listMetaCount } from "../api/page-state";
import { toolsApi } from "../api/tools";
import { useI18n, type MsgKey } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { PageChrome } from "../ui/PageChrome";
import { PageStatus } from "../ui/PageStatus";
import { StatusLine, formatPublicError, redactPublicText } from "../ui/StatusLine";

const emptyForm = { name: "", transport: "mcp-http", endpoint: "", token: "", credential_ref: "", enabled: true };

function formErrKey(err: ConnectorFormError): MsgKey {
  if (err === "needName") return "functions.mcp.needName";
  if (err === "needUrl") return "functions.mcp.needUrl";
  if (err === "needCommand") return "functions.mcp.needCommand";
  return "functions.mcp.badTransport";
}

function healthTone(h?: string): "positive" | "warning" | "critical" | "neutral" {
  const s = (h || "").toLowerCase();
  if (s.includes("ok") || s.includes("healthy") || s === "up") return "positive";
  if (s.includes("fail") || s.includes("error") || s.includes("down")) return "critical";
  if (!h) return "neutral";
  return "warning";
}

export function ConnectorPanel({ variant }: { variant: "mcp" | "connectors" }) {
  const { t, locale } = useI18n();
  const [rows, setRows] = useState<ConnectorInfo[]>([]);
  const [agents, setAgents] = useState<Agent[]>([]);
  const [agentsErr, setAgentsErr] = useState<unknown>(null);
  const [loading, setLoading] = useState(true);
  const [loaded, setLoaded] = useState(false);
  const [loadedAt, setLoadedAt] = useState<string | null>(null);
  const [err, setErr] = useState<unknown>(null);
  const [actionErr, setActionErr] = useState("");
  const [ok, setOk] = useState("");
  const [query, setQuery] = useState("");
  const [showForm, setShowForm] = useState(false);
  const [form, setForm] = useState(emptyForm);
  const [selected, setSelected] = useState("");
  const [endpoint, setEndpoint] = useState("");
  const [token, setToken] = useState("");
  const [testing, setTesting] = useState(false);
  const [testView, setTestView] = useState<ConnectorTestView | null>(null);
  const [agentId, setAgentId] = useState("");
  const [linkName, setLinkName] = useState("");
  const [busy, setBusy] = useState("");

  const state = classifyPageState({
    loading,
    loaded,
    error: err,
    itemCount: rows.length,
    keepStale: loaded && rows.length > 0,
  });
  const blocked = inventoryBlocksMutation(state.kind);
  const visible = useMemo(
    () => filterByQuery(state.showItems ? rows : [], query, (c) => `${c.name} ${c.transport} ${c.endpoint}`),
    [rows, query, state.showItems],
  );
  const filteredEmpty = isFilteredEmpty(state, rows.length, visible.length);
  const metaN = listMetaCount(state.kind, visible.length);
  const current = rows.find((c) => c.name === selected) || null;
  const envLocked = current ? isConnectorEnvOwned(current) : false;
  const agentState = classifyPageState({
    loading: false,
    loaded: agentsErr == null && loaded,
    error: agentsErr,
    itemCount: agents.length,
  });
  const assignBlocked = blocked || inventoryBlocksMutation(agentState.kind);

  async function load() {
    setLoading(true);
    const [cRes, aRes] = await Promise.allSettled([api.listConnectors(), api.listAgents()]);
    const c = resolveSettled(cRes);
    if (c.ok) {
      const next = (c.value.connectors ?? [])
        .map((row) => publicConnector(row as unknown as Record<string, unknown>))
        .filter((row): row is ConnectorInfo => row != null);
      setRows(next);
      setLoaded(true);
      setLoadedAt(new Date().toISOString());
      setErr(null);
      const leak = (c.value.connectors ?? []).some((row) => connectorRowLeaksSecret(row as unknown as Record<string, unknown>));
      setActionErr(leak ? t("functions.mcp.noSecrets") : "");
    } else {
      setErr(c.error);
    }
    const a = resolveSettled(aRes);
    if (a.ok) {
      setAgents(a.value.agents ?? []);
      setAgentsErr(null);
    } else {
      setAgentsErr(a.error);
    }
    setLoading(false);
  }

  useEffect(() => {
    void load();
  }, []);

  useEffect(() => {
    setToken("");
    setTestView(null);
    if (!current) {
      setEndpoint("");
      return;
    }
    setEndpoint(current.endpoint ?? "");
  }, [selected, current?.endpoint]);

  function openCreate() {
    if (blocked) return;
    setShowForm(true);
    setSelected("");
    setForm(emptyForm);
    setActionErr("");
    setOk("");
    setTestView(null);
  }

  async function addConnector() {
    if (blocked) return;
    const fail = connectorFormError(form);
    if (fail) {
      setActionErr(t(formErrKey(fail)));
      return;
    }
    setBusy("create");
    try {
      const created = await toolsApi.createConnector(connectorWriteBody(form));
      setForm(emptyForm);
      setShowForm(false);
      setSelected(created.name);
      setToken("");
      setOk(t("functions.mcp.createdOk", { name: created.name }));
      setActionErr("");
      await load();
    } catch (e) {
      setActionErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  async function saveConnector() {
    if (blocked || !selected || envLocked) return;
    const fail = connectorFormError({ name: selected, transport: current?.transport, endpoint });
    if (fail) {
      setActionErr(t(formErrKey(fail)));
      return;
    }
    setBusy("save");
    try {
      await toolsApi.patchConnector(selected, connectorWriteBody({ endpoint, token, enabled: current?.enabled }));
      setToken("");
      setOk(t("functions.mcp.savedOk", { name: selected }));
      setActionErr("");
      await load();
    } catch (e) {
      setActionErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  async function toggleConnector(c: ConnectorInfo) {
    if (blocked) return;
    setBusy("toggle:" + c.name);
    try {
      await toolsApi.patchConnector(c.name, { enabled: !c.enabled });
      setActionErr("");
      await load();
    } catch (e) {
      setActionErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  async function testConnector(name: string) {
    if (blocked) return;
    const row = rows.find((c) => c.name === name);
    if (!connectorTestReady(row || { name, transport: "http", endpoint: "" })) {
      setActionErr(t("functions.mcp.needEndpoint"));
      return;
    }
    setTesting(true);
    setActionErr("");
    try {
      setTestView(formatConnectorTest(await toolsApi.testConnector(name)));
    } catch (e) {
      setTestView(null);
      setActionErr(formatPublicError(e));
    } finally {
      setTesting(false);
    }
  }

  async function assign() {
    if (assignBlocked) return;
    if (!agentId) {
      setActionErr(t("connectors.needAgent"));
      return;
    }
    if (!linkName) {
      setActionErr(t("connectors.needConnector"));
      return;
    }
    setBusy("assign");
    try {
      await api.linkAgentConnector(agentId, linkName);
      setOk(t("connectors.assignedOk"));
      setActionErr("");
      await load();
    } catch (e) {
      setActionErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  const isMcp = variant === "mcp";
  const formVisible = !blocked && showForm;

  return (
    <PageChrome
      icon="hook"
      title={isMcp ? t("functions.mcp.title") : t("connectors.title")}
      description={isMcp ? t("functions.mcp.desc") : t("connectors.desc")}
      primary={
        <Button variant="primary" icon="plus" disabled={blocked} onClick={openCreate}>
          {isMcp ? t("functions.mcp.add") : t("connectors.register")}
        </Button>
      }
      refresh={
        <Button icon="refresh" iconGesture onClick={() => void load()} disabled={loading}>
          {t("common.refresh")}
        </Button>
      }
      filters={
        state.showItems || state.showEmpty ? (
          <input
            className="z-field"
            style={{ flex: 1, minWidth: 160 }}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={t("functions.mcp.search")}
            aria-label={t("functions.mcp.search")}
            autoComplete="off"
          />
        ) : null
      }
    >
      <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{isMcp ? t("functions.mcp.ownership") : t("connectors.ownership")}</p>
      <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("functions.mcp.noSecrets")}</p>
      <Card>
        <CardHeader icon="lock" title={t("functions.mcp.unavailable")} />
        <ul style={{ margin: 0, padding: "0 16px 14px 32px", fontSize: 12.5, color: "var(--text-3)" }}>
          <li>{t("functions.mcp.unavailable.displayName")}</li>
          <li>{t("functions.mcp.unavailable.args")}</li>
          <li>{t("functions.mcp.unavailable.envValues")}</li>
          <li>{t("functions.mcp.unavailable.hints")}</li>
          <li>{t("functions.mcp.unavailable.prefix")}</li>
          <li>{t("functions.mcp.unavailable.timeout")}</li>
          <li>{t("functions.mcp.unavailable.userCreds")}</li>
          <li>{t("functions.mcp.unavailable.clear")}</li>
        </ul>
      </Card>
      <PageStatus kind={state.kind} errorText={err ? formatPublicError(err) : ""} staleAt={formatStaleAt(loadedAt, locale)} onReload={() => void load()} />
      {actionErr ? <StatusLine kind="error">{actionErr}</StatusLine> : null}
      {ok && !actionErr ? (
        <p role="status" style={{ margin: 0, fontSize: 12.5, color: "var(--green)" }}>
          {ok}
        </p>
      ) : null}
      {formVisible ? (
        <Card>
          <CardHeader icon="plus" title={isMcp ? t("functions.mcp.add") : t("connectors.register")} />
          <div style={{ padding: "0 16px 16px", display: "flex", flexDirection: "column", gap: 10 }}>
            <label style={{ fontSize: 12, color: "var(--text-2)" }}>
              {t("functions.mcp.name")}
              <input className="z-field" style={{ display: "block", width: "100%", marginTop: 4 }} value={form.name} onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))} autoComplete="off" />
            </label>
            <label style={{ fontSize: 12, color: "var(--text-2)" }}>
              {t("functions.mcp.transport")}
              <select className="z-field" style={{ display: "block", width: "100%", marginTop: 4 }} value={form.transport} onChange={(e) => setForm((f) => ({ ...f, transport: e.target.value }))}>
                <option value="http">{t("functions.mcp.http")}</option>
                <option value="mcp-http">{t("functions.mcp.sse")}</option>
                <option value="mcp-stdio">{t("functions.mcp.stdio")}</option>
              </select>
            </label>
            <label style={{ fontSize: 12, color: "var(--text-2)" }}>
              {t("functions.endpoint")}
              <input className="z-field" style={{ display: "block", width: "100%", marginTop: 4 }} value={form.endpoint} onChange={(e) => setForm((f) => ({ ...f, endpoint: e.target.value }))} autoComplete="off" />
            </label>
            <label style={{ fontSize: 12, color: "var(--text-2)" }}>
              {t("functions.token")}
              <input className="z-field" type="password" autoComplete="off" style={{ display: "block", width: "100%", marginTop: 4 }} value={form.token} onChange={(e) => setForm((f) => ({ ...f, token: e.target.value }))} />
            </label>
            <label style={{ fontSize: 12, color: "var(--text-2)" }}>
              {t("functions.mcp.envName")}
              <input className="z-field" style={{ display: "block", width: "100%", marginTop: 4 }} value={form.credential_ref} onChange={(e) => setForm((f) => ({ ...f, credential_ref: e.target.value }))} autoComplete="off" />
            </label>
            <p style={{ margin: 0, fontSize: 12, color: "var(--text-3)" }}>{t("functions.mcp.envHint")}</p>
            <div style={{ display: "flex", gap: 8 }}>
              <Button variant="primary" disabled={Boolean(busy)} onClick={() => void addConnector()}>
                {isMcp ? t("functions.mcp.add") : t("connectors.register")}
              </Button>
              <Button
                variant="quiet"
                onClick={() => {
                  setShowForm(false);
                  setForm(emptyForm);
                }}
              >
                {t("common.cancel")}
              </Button>
            </div>
          </div>
        </Card>
      ) : null}
      <Card>
        <CardHeader icon="hook" title={isMcp ? t("functions.connectors") : t("connectors.list")} meta={metaN == null ? "—" : t("functions.mcp.meta", { n: metaN })} />
        <TableScroll>
          <div style={{ display: "flex", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)", gap: 8 }}>
            <span style={{ flex: 1.2 }}>{t("functions.col.name")}</span>
            <span style={{ flex: 1 }}>{t("functions.mcp.transport")}</span>
            <span style={{ flex: 1.6 }}>{t("functions.endpoint")}</span>
            <span style={{ flex: 1 }}>{t("functions.tokenSet")}</span>
            <span style={{ flex: 0.8 }}>{t("functions.col.on")}</span>
            <span style={{ flex: 0.9 }} />
          </div>
          {state.showItems
            ? visible.map((c) => (
                <div
                  key={c.name}
                  role="button"
                  tabIndex={0}
                  onClick={() => {
                    if (blocked) return;
                    setSelected(c.name);
                    setShowForm(false);
                    setLinkName(c.name);
                  }}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" || e.key === " ") setSelected(c.name);
                  }}
                  style={{
                    display: "flex",
                    width: "100%",
                    textAlign: "left",
                    alignItems: "center",
                    padding: "11px 16px",
                    fontSize: 12.5,
                    borderBottom: "1px solid var(--border-soft)",
                    background: selected === c.name ? "var(--bg-2)" : "transparent",
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
                    {c.health ? <Badge tone={healthTone(c.health)}>{c.health}</Badge> : null}
                  </span>
                  <span style={{ flex: 0.8 }}>
                    <Badge tone={c.enabled ? "positive" : "neutral"}>{c.enabled ? t("common.enabled") : t("common.disabled")}</Badge>
                  </span>
                  <span style={{ flex: 0.9, textAlign: "right" }} onClick={(e) => e.stopPropagation()}>
                    <Button variant="ghost" disabled={blocked} onClick={() => void toggleConnector(c)}>
                      {c.enabled ? t("common.disabled") : t("functions.cron.enable")}
                    </Button>
                  </span>
                </div>
              ))
            : null}
          {state.showEmpty ? <EmptyState data-page-state="empty">{t("functions.emptyConnectors")}</EmptyState> : null}
          {filteredEmpty ? <EmptyState data-page-state="filtered_empty">{t("functions.mcp.filterEmpty")}</EmptyState> : null}
        </TableScroll>
        {current && !blocked ? (
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
            <p style={{ margin: 0, fontSize: 12, color: "var(--text-3)" }}>{t("functions.mcp.testDi")}</p>
            <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
              <Button variant="primary" onClick={() => void saveConnector()} disabled={envLocked || Boolean(busy)}>
                {t("functions.saveConnector")}
              </Button>
              <Button onClick={() => void testConnector(current.name)} disabled={testing || !connectorTestReady({ name: current.name, transport: current.transport, endpoint: endpoint || current.endpoint })}>
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
      {variant === "connectors" ? (
        <Card>
          <CardHeader icon="user-check" title={t("connectors.assign")} />
          {inventoryBlocksMutation(agentState.kind) ? (
            <div data-page-state="dependency" style={{ padding: "0 16px 14px" }}>
              <StatusLine kind="error">
                {t("connectors.agentsFailed")}
                {agentsErr ? ` · ${formatPublicError(agentsErr)}` : ""}
              </StatusLine>
            </div>
          ) : null}
          {agentState.showEmpty ? (
            <EmptyState data-page-state="dependency">{t("connectors.noAgents")}</EmptyState>
          ) : null}
          {!assignBlocked && agentState.showItems ? (
            <div style={{ padding: 14, display: "flex", gap: 8, flexWrap: "wrap" }}>
              <select className="z-field" value={agentId} onChange={(e) => setAgentId(e.target.value)}>
                <option value="">{t("connectors.pickAgent")}</option>
                {agents.map((a) => (
                  <option key={a.id} value={a.id}>
                    {a.display_name || a.agent_key}
                  </option>
                ))}
              </select>
              <select className="z-field" value={linkName} onChange={(e) => setLinkName(e.target.value)}>
                <option value="">{t("connectors.pickConnector")}</option>
                {rows.map((c) => (
                  <option key={c.name} value={c.name}>
                    {c.name}
                  </option>
                ))}
              </select>
              <Button variant="primary" disabled={Boolean(busy)} onClick={() => void assign()}>
                {t("connectors.assignBtn")}
              </Button>
            </div>
          ) : (
            <p style={{ margin: 0, padding: "0 16px 14px", fontSize: 12.5, color: "var(--text-3)" }}>{t("connectors.assignBlocked")}</p>
          )}
        </Card>
      ) : null}
    </PageChrome>
  );
}
