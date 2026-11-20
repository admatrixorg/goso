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
import { Button } from "../ui/Button";
import { Card, CardHeader } from "../ui/Card";
import { DataTable, THead, Th, Td, Tr } from "../ui/DataTable";
import { EmptyState } from "../ui/EmptyState";
import { PageChrome } from "../ui/PageChrome";
import { PageStatus } from "../ui/PageStatus";
import { StatusBadge, type Status } from "../ui/StatusBadge";
import { StatusLine, formatPublicError } from "../ui/StatusLine";
import { fieldClass, labelClass, selectClass } from "../ui/field";

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

  function healthStatus(h: string): Status {
    if (h === "ok") return "success";
    if (h === "unknown") return "warning";
    if (h === "failed" || h === "disconnected") return "error";
    return "default";
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

  return (
    <PageChrome
      icon="monitor"
      title={t("ws.title")}
      description={t("ws.desc")}
      primary={
        <Button icon="plus" variant="default" onClick={openCreate} disabled={blocked || loading || Boolean(busy)}>
          {t("ws.add")}
        </Button>
      }
      refresh={
        <Button icon="refresh" iconGesture variant="outline" onClick={() => void load()} disabled={loading || Boolean(busy)}>
          {t("common.refresh")}
        </Button>
      }
    >
      <Card>
        <CardHeader icon="lock" title={t("ws.how")} />
        <p className="max-w-xl px-4 pb-3 text-sm text-muted-foreground">{t("ws.howBody")}</p>
        <p className="max-w-xl px-4 pb-4 text-sm text-muted-foreground">{t("ws.field.keyNote")}</p>
      </Card>
      <PageStatus kind={state.kind} errorText={err ? formatPublicError(err) : ""} staleAt={formatStaleAt(loadedAt, locale)} onReload={() => void load()} />
      {actionErr ? <StatusLine kind="error">{actionErr}</StatusLine> : null}
      {ok && !actionErr ? (
        <p role="status" className="text-sm text-success">
          {ok}
        </p>
      ) : null}
      {confirm && !blocked ? (
        <Card>
          <CardHeader icon="lock" title={confirm.kind === "disconnect" ? t("ws.confirmDisconnectTitle") : t("ws.confirmDeleteTitle")} />
          <div className="flex flex-col gap-3 p-4">
            <p className="text-sm">
              {confirm.kind === "disconnect"
                ? t("ws.confirmDisconnectPreview", { name: wsLabel(confirm.row) })
                : t("ws.confirmDeletePreview", { name: wsLabel(confirm.row) })}
            </p>
            <p className="text-sm text-muted-foreground">{t("ws.confirmHint")}</p>
            <input
              className={fieldClass}
              value={typed}
              onChange={(e) => setTyped(e.target.value)}
              placeholder={t("ws.confirmPlaceholder")}
              aria-label={t("ws.confirmPlaceholder")}
              autoComplete="off"
              spellCheck={false}
            />
            <div className="flex flex-wrap gap-2">
              <Button variant="destructive" disabled={!matched || Boolean(busy)} onClick={() => void submitConfirm()}>
                {confirm.kind === "disconnect" ? t("ws.confirmDisconnect") : t("ws.confirmDelete")}
              </Button>
              <Button
                variant="outline"
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
          <div className="grid grid-cols-[repeat(auto-fit,minmax(180px,1fr))] gap-3 p-4">
            <label className={labelClass}>
              {t("ws.field.display")}
              <input className={fieldClass + " mt-1"} value={form.display} onChange={(e) => setForm((f) => ({ ...f, display: e.target.value }))} autoComplete="off" />
            </label>
            <label className={labelClass}>
              {t("ws.field.backend")}
              <select
                className={selectClass + " mt-1 w-full"}
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
            <label className={labelClass}>
              {t("ws.field.host")}
              <input className={fieldClass + " mt-1"} value={form.host} onChange={(e) => setForm((f) => ({ ...f, host: e.target.value }))} autoComplete="off" />
            </label>
            <label className={labelClass}>
              {t("ws.field.port")}
              <input className={fieldClass + " mt-1"} value={form.port} onChange={(e) => setForm((f) => ({ ...f, port: e.target.value }))} inputMode="numeric" />
            </label>
            <label className={labelClass}>
              {t("ws.field.user")}
              <input className={fieldClass + " mt-1"} value={form.user} onChange={(e) => setForm((f) => ({ ...f, user: e.target.value }))} autoComplete="off" />
            </label>
            <label className={labelClass + " col-span-full"}>
              {t("ws.field.identity")}
              <input
                className={fieldClass + " mt-1"}
                value={form.identity_ref}
                onChange={(e) => setForm((f) => ({ ...f, identity_ref: e.target.value }))}
                placeholder={t("ws.field.identityHint")}
                autoComplete="off"
                spellCheck={false}
              />
            </label>
            <label className={labelClass}>
              {t("ws.field.agent")}
              <select className={selectClass + " mt-1 w-full"} value={form.agent_id} onChange={(e) => setForm((f) => ({ ...f, agent_id: e.target.value }))}>
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
            <p role="status" className="px-4 pb-2 text-sm text-warning">
              {t(identErr)}
            </p>
          ) : (
            <p className="px-4 pb-2 text-sm text-muted-foreground">{t("ws.testValidation")}</p>
          )}
          {agentsErr ? <p className="px-4 pb-3 text-sm text-muted-foreground">{t("ws.agentsUnavailable")}</p> : null}
          <div className="flex flex-wrap gap-2 px-4 pb-4">
            <Button variant="default" disabled={Boolean(busy)} onClick={() => void save()}>
              {editing ? t("common.save") : t("common.create")}
            </Button>
            {editing && selected ? (
              <Button variant="outline" disabled={Boolean(busy)} onClick={() => void runTest(selected)}>
                {t("ws.test")}
              </Button>
            ) : null}
            <Button
              variant="outline"
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
          <p className="px-4 pb-2 text-sm">
            {testView.summary} · {testView.backend} {testView.host}:{testView.port} · {testView.identity_set ? t("ws.identitySet") : t("ws.identityUnset")}
          </p>
          <p className="px-4 pb-4 text-sm text-muted-foreground">{t("ws.testValidation")}</p>
        </Card>
      ) : null}
      {current && !showForm && state.showItems ? (
        <Card>
          <CardHeader icon="cloud" title={wsLabel(current)} meta={healthLabel(current.health)} />
          <div className="grid gap-1.5 p-4 text-sm">
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
      <div className="flex items-center">
        <h2 className="text-sm font-semibold">{t("ws.list")}</h2>
        <span className="ml-auto text-xs text-muted-foreground">{metaN == null ? "—" : t("ws.list.meta", { n: metaN })}</span>
      </div>
      {state.showEmpty ? <EmptyState icon="monitor" title={t("ws.empty")} /> : null}
      {state.showItems ? (
        <DataTable minWidth="800px" className="mt-0">
          <THead>
            <Th>{t("ws.col.display")}</Th>
            <Th>{t("ws.col.backend")}</Th>
            <Th>{t("ws.col.target")}</Th>
            <Th>{t("ws.col.agent")}</Th>
            <Th>{t("ws.col.health")}</Th>
            <Th className="text-right" />
          </THead>
          <tbody>
            {rows.map((row) => {
              const rowBusy = busy.endsWith(":" + row.id);
              return (
                <Tr key={row.id} selected={selected === row.id} onClick={() => pick(row)}>
                  <Td className="font-semibold">{wsLabel(row)}</Td>
                  <Td className="text-muted-foreground">{row.backend || na}</Td>
                  <Td className="tabular-nums text-muted-foreground">
                    {row.host}:{row.port}
                  </Td>
                  <Td className="text-muted-foreground">{agentLabel(row.agent_id)}</Td>
                  <Td>
                    <StatusBadge status={healthStatus(row.health)} label={healthLabel(row.health)} />
                  </Td>
                  <Td onClick={(e) => e.stopPropagation()}>
                    <div className="flex flex-wrap justify-end gap-2">
                      <Button variant="outline" size="sm" disabled={blocked || rowBusy} onClick={() => pick(row)}>
                        {t("ws.edit")}
                      </Button>
                      <Button variant="outline" size="sm" disabled={blocked || rowBusy} onClick={() => void runTest(row.id)}>
                        {t("ws.test")}
                      </Button>
                      <Button variant="ghost" size="sm" disabled={blocked || rowBusy || row.health === "disconnected"} onClick={() => openConfirm("disconnect", row)}>
                        {t("common.disconnect")}
                      </Button>
                      <Button variant="ghost" size="sm" className="text-destructive" disabled={blocked || rowBusy} onClick={() => openConfirm("delete", row)}>
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
    </PageChrome>
  );
}
