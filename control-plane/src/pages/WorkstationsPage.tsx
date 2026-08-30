import { useEffect, useState } from "react";
import { api, type Agent } from "../api/client";
import { classifyPageState, formatStaleAt, inventoryBlocksMutation, listMetaCount } from "../api/page-state";
import { workstationsApi, type Workstation, type WorkstationTest } from "../api/workstations";
import {
  asPublic,
  asPublicTest,
  formatWhen,
  publicHasSecrets,
  testOutcome,
  writeBody,
  wsConfirmMatch,
  wsFormError,
  wsLabel,
} from "../api/workstations-ops";
import { useI18n } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { PageChrome } from "../ui/PageChrome";
import { PageStatus } from "../ui/PageStatus";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

type ActionKind = "disconnect" | "delete";
const emptyForm = { display: "", backend: "ssh", host: "", port: "22", user: "", identity_ref: "", agent_id: "" };

export function WorkstationsPage() {
  const { t, locale } = useI18n();
  const [rows, setRows] = useState<Workstation[]>([]);
  const [agents, setAgents] = useState<Agent[]>([]);
  const [agentsErr, setAgentsErr] = useState<unknown>(null);
  const [loading, setLoading] = useState(true);
  const [loaded, setLoaded] = useState(false);
  const [loadedAt, setLoadedAt] = useState<string | null>(null);
  const [err, setErr] = useState<unknown>(null);
  const [actionErr, setActionErr] = useState("");
  const [ok, setOk] = useState("");
  const [busy, setBusy] = useState("");
  const [selected, setSelected] = useState("");
  const [form, setForm] = useState(emptyForm);
  const [editing, setEditing] = useState(false);
  const [showForm, setShowForm] = useState(false);
  const [testView, setTestView] = useState<WorkstationTest | null>(null);
  const [confirm, setConfirm] = useState<{ kind: ActionKind; row: Workstation } | null>(null);
  const [typed, setTyped] = useState("");
  const na = t("ws.na");
  const state = classifyPageState({
    loading,
    loaded,
    error: err,
    itemCount: rows.length,
    keepStale: loaded && rows.length > 0,
  });
  const blocked = inventoryBlocksMutation(state.kind);
  const formVisible = !blocked && showForm;
  const identErr = wsFormError(form);
  const matched = confirm ? wsConfirmMatch(typed, confirm.row) : false;
  const metaN = listMetaCount(state.kind, rows.length);
  const current = rows.find((r) => r.id === selected) || null;
  const outcome = testOutcome(testView);

  async function load() {
    setLoading(true);
    const [wsRes, agRes] = await Promise.allSettled([workstationsApi.list(), api.listAgents()]);
    if (wsRes.status === "fulfilled") {
      const next = asPublic(wsRes.value.workstations);
      setRows(next);
      setLoaded(true);
      setLoadedAt(new Date().toISOString());
      const leak = next.some((row) => publicHasSecrets(row)) || (wsRes.value.workstations || []).some((row) => publicHasSecrets(row));
      setActionErr(leak ? t("ws.leak") : "");
      setErr(null);
    } else {
      setErr(wsRes.reason);
    }
    if (agRes.status === "fulfilled") {
      setAgents(agRes.value.agents || []);
      setAgentsErr(null);
    } else {
      setAgentsErr(agRes.reason);
    }
    setLoading(false);
  }

  useEffect(() => {
    void load();
  }, []);

  function healthTone(h: string): "neutral" | "accent" | "positive" | "warning" | "critical" {
    if (h === "ok") return "positive";
    if (h === "unknown") return "warning";
    if (h === "failed" || h === "disconnected") return "critical";
    return "neutral";
  }

  function healthLabel(h: string): string {
    if (h === "ok") return t("ws.health.ok");
    if (h === "unknown") return t("ws.health.unknown");
    if (h === "failed") return t("ws.health.failed");
    if (h === "disconnected") return t("ws.health.disconnected");
    return h || na;
  }

  function agentLabel(id: string | undefined): string {
    const v = (id || "").trim();
    if (!v) return na;
    const a = agents.find((x) => x.id === v);
    return a?.display_name || a?.agent_key || v;
  }

  function openCreate() {
    if (blocked) return;
    setSelected("");
    setEditing(false);
    setShowForm(true);
    setForm(emptyForm);
    setTestView(null);
    setConfirm(null);
    setOk("");
    setActionErr("");
  }

  function pick(row: Workstation) {
    if (blocked) return;
    setSelected(row.id);
    setEditing(true);
    setShowForm(true);
    setForm({
      display: row.display || "",
      backend: row.backend || "ssh",
      host: row.host || "",
      port: row.port ? String(row.port) : "",
      user: row.user || "",
      identity_ref: row.identity_ref || "",
      agent_id: row.agent_id || "",
    });
    setTestView(null);
    setConfirm(null);
    setOk("");
    setActionErr("");
  }

  function openConfirm(kind: ActionKind, row: Workstation) {
    if (blocked) return;
    setConfirm({ kind, row });
    setTyped("");
    setOk("");
    setActionErr("");
  }

  async function save() {
    if (blocked) return;
    if (identErr) {
      setActionErr(t(identErr));
      return;
    }
    setBusy("save");
    try {
      const body = writeBody(form);
      if (editing && selected) {
        await workstationsApi.patch(selected, body);
        setOk(t("ws.saveOk"));
      } else {
        const created = await workstationsApi.create(body);
        setSelected(created.id);
        setEditing(true);
        setOk(t("ws.createOk"));
      }
      setActionErr("");
      await load();
    } catch (e) {
      setActionErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  async function runTest(id: string) {
    if (blocked) return;
    setBusy("test:" + id);
    try {
      const raw = await workstationsApi.test(id);
      const next = asPublicTest(raw);
      if (!next) {
        setActionErr(t("ws.leak"));
        setTestView(null);
        return;
      }
      setTestView(next);
      setOk(next.ok ? t("ws.testOk") : t("ws.testFail"));
      setActionErr("");
      await load();
    } catch (e) {
      setActionErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  async function submitConfirm() {
    if (!confirm || blocked) return;
    if (!wsConfirmMatch(typed, confirm.row)) {
      setActionErr(t("ws.mismatch"));
      return;
    }
    const name = typed.trim();
    const kind = confirm.kind;
    setBusy(`${kind}:${confirm.row.id}`);
    try {
      if (kind === "disconnect") await workstationsApi.disconnect(confirm.row.id, name);
      else await workstationsApi.remove(confirm.row.id, name);
      setOk(kind === "disconnect" ? t("ws.disconnectOk") : t("ws.deleteOk"));
      setActionErr("");
      setConfirm(null);
      setTyped("");
      if (kind === "delete" && selected === confirm.row.id) {
        setSelected("");
        setShowForm(false);
        setForm(emptyForm);
        setEditing(false);
      }
      setTestView(null);
      await load();
    } catch (e) {
      setActionErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  const fieldStyle = { display: "flex", flexDirection: "column" as const, gap: 4, fontSize: 12.5, color: "var(--text-2)" };

  return (
    <PageChrome
      icon="cloud"
      title={t("ws.title")}
      description={t("ws.desc")}
      primary={
        <Button icon="plus" variant="primary" onClick={openCreate} disabled={blocked || loading || Boolean(busy)}>
          {t("ws.add")}
        </Button>
      }
      refresh={
        <Button icon="refresh" iconGesture onClick={() => void load()} disabled={loading || Boolean(busy)}>
          {t("common.refresh")}
        </Button>
      }
    >
      <Card>
        <CardHeader icon="lock" title={t("ws.how")} />
        <p style={{ margin: 0, padding: "0 16px 14px", fontSize: 12.5, color: "var(--text-3)", maxWidth: 720 }}>
          {t("ws.howBody")}
        </p>
        <p style={{ margin: 0, padding: "0 16px 14px", fontSize: 12.5, color: "var(--text-3)", maxWidth: 720 }}>
          {t("ws.field.keyNote")}
        </p>
      </Card>
      <PageStatus kind={state.kind} errorText={err ? formatPublicError(err) : ""} staleAt={formatStaleAt(loadedAt, locale)} onReload={() => void load()} />
      {actionErr ? <StatusLine kind="error">{actionErr}</StatusLine> : null}
      {ok && !actionErr ? (
        <p role="status" style={{ margin: 0, fontSize: 12.5, color: "var(--green)" }}>
          {ok}
        </p>
      ) : null}
      {confirm && !blocked ? (
        <Card>
          <CardHeader icon="lock" title={confirm.kind === "disconnect" ? t("ws.confirmDisconnectTitle") : t("ws.confirmDeleteTitle")} />
          <div style={{ padding: "0 16px 16px", display: "flex", flexDirection: "column", gap: 10 }}>
            <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-2)" }}>
              {confirm.kind === "disconnect"
                ? t("ws.confirmDisconnectPreview", { name: wsLabel(confirm.row) })
                : t("ws.confirmDeletePreview", { name: wsLabel(confirm.row) })}
            </p>
            <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("ws.confirmHint")}</p>
            <input
              className="z-field"
              value={typed}
              onChange={(e) => setTyped(e.target.value)}
              placeholder={t("ws.confirmPlaceholder")}
              aria-label={t("ws.confirmPlaceholder")}
              autoComplete="off"
              spellCheck={false}
            />
            <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
              <Button
                variant="primary"
                disabled={!matched || Boolean(busy)}
                onClick={() => void submitConfirm()}
                style={{ background: "var(--red)", borderColor: "transparent" }}
              >
                {confirm.kind === "disconnect" ? t("ws.confirmDisconnect") : t("ws.confirmDelete")}
              </Button>
              <Button
                variant="quiet"
                disabled={Boolean(busy)}
                onClick={() => {
                  setConfirm(null);
                  setTyped("");
                }}
              >
                {t("ws.cancel")}
              </Button>
            </div>
          </div>
        </Card>
      ) : null}
      {formVisible ? (
        <Card>
          <CardHeader icon="build" title={editing ? t("ws.edit") : t("ws.create")} />
          <div style={{ padding: "0 16px 16px", display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(180px, 1fr))", gap: 10 }}>
            <label style={fieldStyle}>
              {t("ws.field.display")}
              <input className="z-field" value={form.display} onChange={(e) => setForm((f) => ({ ...f, display: e.target.value }))} autoComplete="off" />
            </label>
            <label style={fieldStyle}>
              {t("ws.field.backend")}
              <select
                className="z-field"
                value={form.backend}
                onChange={(e) => {
                  const backend = e.target.value;
                  setForm((f) => ({
                    ...f,
                    backend,
                    port: backend === "docker" && (f.port === "22" || !f.port) ? "2375" : backend === "ssh" && (f.port === "2375" || !f.port) ? "22" : f.port,
                  }));
                }}
              >
                <option value="ssh">{t("ws.backend.ssh")}</option>
                <option value="docker">{t("ws.backend.docker")}</option>
              </select>
            </label>
            <label style={fieldStyle}>
              {t("ws.field.host")}
              <input className="z-field" value={form.host} onChange={(e) => setForm((f) => ({ ...f, host: e.target.value }))} autoComplete="off" />
            </label>
            <label style={fieldStyle}>
              {t("ws.field.port")}
              <input className="z-field" value={form.port} onChange={(e) => setForm((f) => ({ ...f, port: e.target.value }))} inputMode="numeric" />
            </label>
            <label style={fieldStyle}>
              {t("ws.field.user")}
              <input className="z-field" value={form.user} onChange={(e) => setForm((f) => ({ ...f, user: e.target.value }))} autoComplete="off" />
            </label>
            <label style={{ ...fieldStyle, gridColumn: "1 / -1" }}>
              {t("ws.field.identity")}
              <input
                className="z-field"
                value={form.identity_ref}
                onChange={(e) => setForm((f) => ({ ...f, identity_ref: e.target.value }))}
                placeholder={t("ws.field.identityHint")}
                autoComplete="off"
                spellCheck={false}
              />
            </label>
            <label style={fieldStyle}>
              {t("ws.field.agent")}
              <select className="z-field" value={form.agent_id} onChange={(e) => setForm((f) => ({ ...f, agent_id: e.target.value }))}>
                <option value="">{t("ws.field.agentNone")}</option>
                {agents.map((a) => (
                  <option key={a.id} value={a.id}>
                    {a.display_name || a.agent_key || a.id}
                  </option>
                ))}
              </select>
            </label>
          </div>
          {identErr ? (
            <p role="status" style={{ margin: 0, padding: "0 16px 8px", fontSize: 12.5, color: "var(--orange)" }}>
              {t(identErr)}
            </p>
          ) : (
            <p style={{ margin: 0, padding: "0 16px 8px", fontSize: 12.5, color: "var(--text-3)" }}>{t("ws.testValidation")}</p>
          )}
          {agentsErr ? (
            <p style={{ margin: 0, padding: "0 16px 12px", fontSize: 12.5, color: "var(--text-3)" }}>{t("ws.agentsUnavailable")}</p>
          ) : null}
          <div style={{ padding: "0 16px 16px", display: "flex", gap: 8, flexWrap: "wrap" }}>
            <Button variant="accent" disabled={Boolean(busy)} onClick={() => void save()}>
              {editing ? t("common.save") : t("common.create")}
            </Button>
            {editing && selected ? (
              <Button variant="quiet" disabled={Boolean(busy)} onClick={() => void runTest(selected)}>
                {t("ws.test")}
              </Button>
            ) : null}
            <Button
              variant="quiet"
              disabled={Boolean(busy)}
              onClick={() => {
                setShowForm(false);
                setEditing(false);
              }}
            >
              {t("ws.cancel")}
            </Button>
          </div>
        </Card>
      ) : null}
      {testView && !blocked ? (
        <Card>
          <CardHeader icon="pulse" title={t("ws.testResult")} meta={outcome === "valid" ? t("ws.testOk") : t("ws.testFail")} />
          <p style={{ margin: 0, padding: "0 16px 8px", fontSize: 12.5, color: "var(--text-2)" }}>
            {testView.summary} · {testView.backend} {testView.host}:{testView.port} · {testView.identity_set ? t("ws.identitySet") : t("ws.identityUnset")}
          </p>
          <p style={{ margin: 0, padding: "0 16px 14px", fontSize: 12.5, color: "var(--text-3)" }}>{t("ws.testValidation")}</p>
        </Card>
      ) : null}
      {current && !showForm && state.showItems ? (
        <Card>
          <CardHeader icon="cloud" title={wsLabel(current)} meta={healthLabel(current.health)} />
          <div style={{ padding: "0 16px 16px", display: "grid", gap: 6, fontSize: 12.5, color: "var(--text-2)" }}>
            <div>
              {t("ws.field.backend")}: {current.backend} · {current.host}:{current.port}
              {current.user ? ` · ${current.user}` : ""}
            </div>
            <div>
              {t("ws.field.identity")}: {current.identity_set ? current.identity_ref || t("ws.identitySet") : t("ws.identityUnset")}
            </div>
            <div>
              {t("ws.field.agent")}: {agentLabel(current.agent_id)}
            </div>
            <div>
              {t("ws.col.tested")}: {formatWhen(current.last_tested, na)}
            </div>
          </div>
        </Card>
      ) : null}
      <Card>
        <CardHeader icon="cloud" title={t("ws.list")} meta={metaN == null ? "—" : t("ws.list.meta", { n: metaN })} />
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
            <span style={{ flex: 1.4 }}>{t("ws.col.display")}</span>
            <span style={{ flex: 1 }}>{t("ws.col.backend")}</span>
            <span style={{ flex: 1.6 }}>{t("ws.col.target")}</span>
            <span style={{ flex: 1.2 }}>{t("ws.col.agent")}</span>
            <span style={{ flex: 1 }}>{t("ws.col.health")}</span>
            <span style={{ flex: 1.8 }} />
          </div>
          {state.showEmpty ? <EmptyState>{t("ws.empty")}</EmptyState> : null}
          {state.showItems
            ? rows.map((row) => {
                const rowBusy = busy.endsWith(":" + row.id);
                return (
                  <div
                    key={row.id}
                    style={{
                      display: "flex",
                      alignItems: "center",
                      padding: "11px 16px",
                      fontSize: 12.5,
                      borderBottom: "1px solid var(--border-soft)",
                      gap: 8,
                      background: selected === row.id ? "var(--bg-2)" : undefined,
                    }}
                  >
                    <button
                      type="button"
                      onClick={() => pick(row)}
                      disabled={blocked}
                      style={{
                        flex: 1.4,
                        fontWeight: 600,
                        textAlign: "left",
                        background: "none",
                        border: 0,
                        color: "inherit",
                        cursor: blocked ? "default" : "pointer",
                        padding: 0,
                      }}
                    >
                      {wsLabel(row)}
                    </button>
                    <span style={{ flex: 1, color: "var(--text-2)" }}>{row.backend || na}</span>
                    <span style={{ flex: 1.6, color: "var(--text-3)", fontVariantNumeric: "tabular-nums" }}>
                      {row.host}:{row.port}
                    </span>
                    <span style={{ flex: 1.2, color: "var(--text-2)" }}>{agentLabel(row.agent_id)}</span>
                    <span style={{ flex: 1 }}>
                      <Badge tone={healthTone(row.health)}>{healthLabel(row.health)}</Badge>
                    </span>
                    <span style={{ flex: 1.8, display: "flex", gap: 6, justifyContent: "flex-end", flexWrap: "wrap" }}>
                      <Button variant="quiet" disabled={blocked || rowBusy} onClick={() => pick(row)}>
                        {t("ws.edit")}
                      </Button>
                      <Button variant="quiet" disabled={blocked || rowBusy} onClick={() => void runTest(row.id)}>
                        {t("ws.test")}
                      </Button>
                      <Button variant="quiet" disabled={blocked || rowBusy || row.health === "disconnected"} onClick={() => openConfirm("disconnect", row)}>
                        {t("common.disconnect")}
                      </Button>
                      <Button variant="quiet" disabled={blocked || rowBusy} onClick={() => openConfirm("delete", row)}>
                        {t("common.delete")}
                      </Button>
                    </span>
                  </div>
                );
              })
            : null}
        </TableScroll>
      </Card>
    </PageChrome>
  );
}
