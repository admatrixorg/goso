import { useEffect, useState } from "react";
import { api, type Agent } from "../api/client";
import { workstationsApi, type Workstation, type WorkstationTest } from "../api/workstations";
import {
  asPublic,
  asPublicTest,
  formatWhen,
  identityError,
  publicHasSecrets,
  writeBody,
  wsConfirmMatch,
  wsLabel,
} from "../api/workstations-ops";
import { useI18n } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { SectionHeader } from "../ui/SectionHeader";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

type ActionKind = "disconnect" | "delete";
const emptyForm = { display: "", backend: "ssh", host: "", port: "22", user: "", identity_ref: "", agent_id: "" };

export function WorkstationsPage() {
  const { t } = useI18n();
  const [rows, setRows] = useState<Workstation[]>([]);
  const [agents, setAgents] = useState<Agent[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState("");
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

  async function load() {
    setLoading(true);
    try {
      const [j, ag] = await Promise.all([
        workstationsApi.list(),
        api.listAgents().catch(() => ({ agents: [] as Agent[] })),
      ]);
      const next = asPublic(j.workstations);
      setRows(next);
      setAgents(ag.agents || []);
      const leak = next.some((row) => publicHasSecrets(row)) || (j.workstations || []).some((row) => publicHasSecrets(row));
      setErr(leak ? t("ws.leak") : "");
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  const current = rows.find((r) => r.id === selected) || null;
  const identErr = identityError(form.identity_ref);
  const matched = confirm ? wsConfirmMatch(typed, confirm.row) : false;

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
    setSelected("");
    setEditing(false);
    setShowForm(true);
    setForm(emptyForm);
    setTestView(null);
    setConfirm(null);
    setOk("");
    setErr("");
  }

  function pick(row: Workstation) {
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
    setErr("");
  }

  function openConfirm(kind: ActionKind, row: Workstation) {
    setConfirm({ kind, row });
    setTyped("");
    setOk("");
    setErr("");
  }

  async function save() {
    if (identErr) {
      setErr(t(identErr));
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
      setErr("");
      await load();
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  async function runTest(id: string) {
    setBusy("test:" + id);
    try {
      const raw = await workstationsApi.test(id);
      const next = asPublicTest(raw);
      if (!next) {
        setErr(t("ws.leak"));
        setTestView(null);
        return;
      }
      setTestView(next);
      setOk(next.ok ? t("ws.testOk") : t("ws.testFail"));
      setErr("");
      await load();
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  async function submitConfirm() {
    if (!confirm) return;
    if (!wsConfirmMatch(typed, confirm.row)) {
      setErr(t("ws.mismatch"));
      return;
    }
    const name = typed.trim();
    const kind = confirm.kind;
    setBusy(`${kind}:${confirm.row.id}`);
    try {
      if (kind === "disconnect") await workstationsApi.disconnect(confirm.row.id, name);
      else await workstationsApi.remove(confirm.row.id, name);
      setOk(kind === "disconnect" ? t("ws.disconnectOk") : t("ws.deleteOk"));
      setErr("");
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
      setErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  const fieldStyle = { display: "flex", flexDirection: "column" as const, gap: 4, fontSize: 12.5, color: "var(--text-2)" };

  return (
    <div style={{ padding: "14px 22px 40px", display: "flex", flexDirection: "column", gap: 14 }}>
      <SectionHeader
        icon="cloud"
        title={t("ws.title")}
        description={t("ws.desc")}
        actions={
          <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
            <Button icon="plus" onClick={openCreate} disabled={loading || Boolean(busy)}>
              {t("ws.add")}
            </Button>
            <Button icon="refresh" iconGesture onClick={() => void load()} disabled={loading || Boolean(busy)}>
              {t("common.refresh")}
            </Button>
          </div>
        }
      />
      <Card>
        <CardHeader icon="lock" title={t("ws.how")} />
        <p style={{ margin: 0, padding: "0 16px 14px", fontSize: 12.5, color: "var(--text-3)", maxWidth: 720 }}>
          {t("ws.howBody")}
        </p>
      </Card>
      {loading ? <StatusLine kind="loading" /> : null}
      {err ? <StatusLine kind="error">{err}</StatusLine> : null}
      {ok && !err ? (
        <p role="status" style={{ margin: 0, fontSize: 12.5, color: "var(--green)" }}>
          {ok}
        </p>
      ) : null}
      {confirm ? (
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
              <Button variant="accent" disabled={!matched || Boolean(busy)} onClick={() => void submitConfirm()}>
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
      {showForm ? (
        <Card>
          <CardHeader icon="build" title={editing ? t("ws.edit") : t("ws.create")} />
          <div style={{ padding: "0 16px 16px", display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(180px, 1fr))", gap: 10 }}>
            <label style={fieldStyle}>
              {t("ws.field.display")}
              <input className="z-field" value={form.display} onChange={(e) => setForm((f) => ({ ...f, display: e.target.value }))} />
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
          <div style={{ padding: "0 16px 16px", display: "flex", gap: 8, flexWrap: "wrap" }}>
            <Button variant="accent" disabled={Boolean(busy) || Boolean(identErr)} onClick={() => void save()}>
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
      {testView ? (
        <Card>
          <CardHeader icon="pulse" title={t("ws.testResult")} />
          <p style={{ margin: 0, padding: "0 16px 14px", fontSize: 12.5, color: "var(--text-2)" }}>
            {testView.summary} · {testView.backend} {testView.host}:{testView.port} · {testView.identity_set ? t("ws.identitySet") : t("ws.identityUnset")}
          </p>
        </Card>
      ) : null}
      {current && !showForm ? (
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
        <CardHeader icon="cloud" title={t("ws.list")} meta={t("ws.list.meta", { n: rows.length })} />
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
          {!loading && rows.length === 0 ? <EmptyState>{t("ws.empty")}</EmptyState> : null}
          {rows.map((row) => {
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
                  style={{
                    flex: 1.4,
                    fontWeight: 600,
                    textAlign: "left",
                    background: "none",
                    border: 0,
                    color: "inherit",
                    cursor: "pointer",
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
                  <Button variant="quiet" disabled={rowBusy} onClick={() => pick(row)}>
                    {t("ws.edit")}
                  </Button>
                  <Button variant="quiet" disabled={rowBusy} onClick={() => void runTest(row.id)}>
                    {t("ws.test")}
                  </Button>
                  <Button variant="quiet" disabled={rowBusy || row.health === "disconnected"} onClick={() => openConfirm("disconnect", row)}>
                    {t("common.disconnect")}
                  </Button>
                  <Button variant="quiet" disabled={rowBusy} onClick={() => openConfirm("delete", row)}>
                    {t("common.delete")}
                  </Button>
                </span>
              </div>
            );
          })}
        </TableScroll>
      </Card>
    </div>
  );
}
