import { useEffect, useMemo, useState } from "react";
import {
  agentConflictKind,
  agentDisplayName,
  filterAgents,
  isAgentActive,
  uniqueProviders,
  validateAgentKey,
  type AgentStatusFilter,
} from "../api/agents";
import { api, ORCHESTRATION_MODES, type Agent } from "../api/client";
import { confirmNamed } from "../api/confirm";
import { classifyPageState } from "../api/page-state";
import { providersApi, type ProviderInfo } from "../api/providers";
import { useI18n, type MsgKey } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { PageChrome } from "../ui/PageChrome";
import { PageStatus } from "../ui/PageStatus";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

function modeLabelKey(mode: string): MsgKey {
  if (mode === "auto") return "agents.mode.auto";
  if (mode === "explicit") return "agents.mode.explicit";
  if (mode === "manual") return "agents.mode.manual";
  return "agents.mode.unset";
}

const emptyForm = { key: "", name: "", model: "", llm_provider: "", instructions: "", mode: "", enabled: true };

function formFromAgent(a: Agent) {
  return {
    key: a.agent_key,
    name: a.display_name,
    model: a.model || "",
    llm_provider: a.llm_provider || "",
    instructions: a.instructions || "",
    mode: a.orchestration_mode || "",
    enabled: isAgentActive(a),
  };
}

export function AgentsPage() {
  const { t } = useI18n();
  const [agents, setAgents] = useState<Agent[]>([]);
  const [providers, setProviders] = useState<ProviderInfo[]>([]);
  const [err, setErr] = useState<unknown>(null);
  const [formErr, setFormErr] = useState("");
  const [loaded, setLoaded] = useState(false);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [loadingDetail, setLoadingDetail] = useState(false);
  const [selectedId, setSelectedId] = useState("");
  const [updatedAt, setUpdatedAt] = useState("");
  const [form, setForm] = useState(emptyForm);
  const [query, setQuery] = useState("");
  const [statusFilter, setStatusFilter] = useState<AgentStatusFilter>("");
  const [providerFilter, setProviderFilter] = useState("");
  const [createOpen, setCreateOpen] = useState(false);
  const [conflict, setConflict] = useState(false);

  const editing = Boolean(selectedId);
  const formVisible = editing || createOpen;
  const busy = saving || deleting || loadingDetail;
  const state = classifyPageState({
    loading,
    loaded,
    error: err,
    itemCount: agents.length,
    keepStale: loaded && agents.length > 0,
  });

  const visible = useMemo(
    () => filterAgents(agents, { query, status: statusFilter, provider: providerFilter }),
    [agents, query, statusFilter, providerFilter],
  );
  const providerOptions = useMemo(() => uniqueProviders(agents), [agents]);
  const filteredEmpty = state.showItems && agents.length > 0 && visible.length === 0;

  async function load() {
    setLoading(true);
    try {
      const [j, p] = await Promise.allSettled([api.listAgents(), providersApi.list()]);
      if (j.status === "fulfilled") {
        setAgents(j.value.agents ?? []);
        setLoaded(true);
        setErr(null);
      } else {
        setErr(j.reason);
      }
      if (p.status === "fulfilled") {
        setProviders(p.value.providers ?? []);
      }
    } catch (e) {
      setErr(e);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  function applyDetail(a: Agent) {
    setSelectedId(a.id);
    setForm(formFromAgent(a));
    setUpdatedAt((a.updated_at || a.created_at || "").trim());
    setCreateOpen(false);
    setConflict(false);
  }

  async function pick(a: Agent) {
    if (busy) return;
    setFormErr("");
    setLoadingDetail(true);
    try {
      const detail = await api.getAgent(a.id);
      applyDetail(detail);
    } catch (e) {
      setFormErr(mapErr(e));
    } finally {
      setLoadingDetail(false);
    }
  }

  function resetForm() {
    setSelectedId("");
    setUpdatedAt("");
    setForm(emptyForm);
    setFormErr("");
    setCreateOpen(false);
    setConflict(false);
  }

  function mapErr(e: unknown): string {
    const kind = agentConflictKind(e);
    if (kind === "lead") return t("agents.cannotDeleteLead");
    if (kind === "inactive") return t("agents.inactive");
    if (kind === "exists") return t("agents.keyExists");
    if (kind === "conflict") {
      setConflict(true);
      return t("agents.conflict");
    }
    return formatPublicError(e);
  }

  async function reloadSelected() {
    if (!selectedId) return;
    setLoadingDetail(true);
    setFormErr("");
    try {
      const detail = await api.getAgent(selectedId);
      applyDetail(detail);
    } catch (e) {
      setFormErr(mapErr(e));
    } finally {
      setLoadingDetail(false);
    }
  }

  async function save() {
    setFormErr("");
    const keyErr = validateAgentKey(form.key, editing);
    if (keyErr) {
      setFormErr(t(keyErr));
      return;
    }
    if (!editing) {
      setSaving(true);
      try {
        const created = await api.createAgent({
          agent_key: form.key.trim(),
          display_name: form.name.trim() || form.key.trim(),
          model: form.model.trim() || undefined,
          llm_provider: form.llm_provider.trim() || undefined,
          instructions: form.instructions.trim() || undefined,
          orchestration_mode: form.mode || undefined,
          enabled: form.enabled,
        });
        applyDetail(created);
        await load();
      } catch (e) {
        setFormErr(mapErr(e));
      } finally {
        setSaving(false);
      }
      return;
    }

    const body: {
      model?: string;
      llm_provider?: string;
      instructions?: string;
      orchestration_mode?: string;
      enabled: boolean;
      if_updated_at?: string;
    } = {
      model: form.model.trim(),
      llm_provider: form.llm_provider.trim(),
      instructions: form.instructions.trim(),
      enabled: form.enabled,
    };
    if (ORCHESTRATION_MODES.includes(form.mode as (typeof ORCHESTRATION_MODES)[number])) {
      body.orchestration_mode = form.mode;
    }
    if (updatedAt) body.if_updated_at = updatedAt;
    setSaving(true);
    try {
      const updated = await api.updateAgent(selectedId, body);
      applyDetail(updated);
      await load();
    } catch (e) {
      setFormErr(mapErr(e));
    } finally {
      setSaving(false);
    }
  }

  async function remove() {
    if (!editing || busy) return;
    const current = agents.find((a) => a.id === selectedId);
    const named = agentDisplayName(current || { id: selectedId, display_name: form.name, agent_key: form.key });
    if (!confirmNamed(t("agents.confirmDelete", { name: named }), (m) => window.confirm(m))) return;
    setDeleting(true);
    try {
      await api.deleteAgent(selectedId);
      resetForm();
      await load();
    } catch (e) {
      setFormErr(mapErr(e));
    } finally {
      setDeleting(false);
    }
  }

  return (
    <PageChrome
      icon="bolt"
      title={t("agents.title")}
      description={t("agents.desc")}
      primary={
        <Button
          variant="primary"
          icon="plus"
          onClick={() => {
            setSelectedId("");
            setForm(emptyForm);
            setCreateOpen(true);
            setFormErr("");
            setConflict(false);
          }}
        >
          {t("agents.create")}
        </Button>
      }
      refresh={
        <Button icon="refresh" iconGesture onClick={() => void load()}>
          {t("common.refresh")}
        </Button>
      }
      filters={
        <>
          <input
            className="z-field"
            style={{ flex: 1, minWidth: 160 }}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={t("agents.search")}
            aria-label={t("agents.search")}
            autoComplete="off"
          />
          <select
            className="z-field"
            aria-label={t("agents.filterStatus")}
            value={statusFilter}
            onChange={(e) => setStatusFilter(e.target.value as AgentStatusFilter)}
            style={{ minWidth: 140 }}
          >
            <option value="">{t("agents.filterStatusAll")}</option>
            <option value="active">{t("agents.status.active")}</option>
            <option value="inactive">{t("agents.status.inactive")}</option>
          </select>
          <select
            className="z-field"
            aria-label={t("agents.filterProvider")}
            value={providerFilter}
            onChange={(e) => setProviderFilter(e.target.value)}
            style={{ minWidth: 140 }}
          >
            <option value="">{t("agents.filterProviderAll")}</option>
            {providerOptions.map((p) => (
              <option key={p} value={p}>
                {p}
              </option>
            ))}
          </select>
        </>
      }
    >
      <Badge tone="neutral" data-agent-transfer="unavailable">
        {t("agents.transferUnavailable")}
      </Badge>
      <PageStatus kind={state.kind} errorText={err ? formatPublicError(err) : ""} onReload={() => void load()} />
      {formErr ? <StatusLine kind="error">{formErr}</StatusLine> : null}
      {conflict ? (
        <div role="status" data-agent-conflict="" style={{ display: "flex", gap: 8, alignItems: "center", flexWrap: "wrap" }}>
          <Button icon="refresh" onClick={() => void reloadSelected()}>
            {t("agents.conflictReload")}
          </Button>
        </div>
      ) : null}
      <Card>
        <CardHeader icon="user" title={t("agents.list")} meta={state.showItems ? t("agents.meta", { n: visible.length }) : "—"} />
        <TableScroll>
        <div style={{ display: "flex", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)", gap: 8 }}>
          <span style={{ flex: 1.4 }}>{t("agents.col.key")}</span>
          <span style={{ flex: 2 }}>{t("agents.col.name")}</span>
          <span style={{ flex: 1.1 }}>{t("agents.col.status")}</span>
          <span style={{ flex: 1.2 }}>{t("agents.col.provider")}</span>
          <span style={{ flex: 1.2 }}>{t("agents.col.model")}</span>
          <span style={{ flex: 1.4 }}>{t("agents.col.mode")}</span>
        </div>
        {state.showItems
          ? visible.map((a) => {
              const on = selectedId === a.id;
              const active = isAgentActive(a);
              return (
                <button
                  key={a.id}
                  type="button"
                  onClick={() => {
                    void pick(a);
                  }}
                  style={{
                    display: "flex",
                    width: "100%",
                    textAlign: "left",
                    padding: "11px 16px",
                    fontSize: 12.5,
                    border: "none",
                    borderBottom: "1px solid var(--border-soft)",
                    background: on ? "var(--accent-soft)" : "transparent",
                    color: "var(--text)",
                    gap: 8,
                    cursor: busy ? "default" : "pointer",
                    alignItems: "center",
                  }}
                >
                  <span style={{ flex: 1.4, fontWeight: 600 }}>{a.agent_key}</span>
                  <span style={{ flex: 2 }}>{a.display_name}</span>
                  <span style={{ flex: 1.1 }}>
                    <Badge tone={active ? "positive" : "neutral"}>{active ? t("agents.status.active") : t("agents.status.inactive")}</Badge>
                  </span>
                  <span style={{ flex: 1.2, color: "var(--text-2)" }}>{a.llm_provider || t("agents.provider.default")}</span>
                  <span style={{ flex: 1.2, color: "var(--text-2)" }}>{a.model || "—"}</span>
                  <span style={{ flex: 1.4 }}>{t(modeLabelKey(a.orchestration_mode || ""))}</span>
                </button>
              );
            })
          : null}
        {loadingDetail ? <StatusLine kind="loading" /> : null}
        {state.showEmpty ? <EmptyState>{t("agents.empty")}</EmptyState> : null}
        {filteredEmpty ? <EmptyState>{t("agents.emptyFilter")}</EmptyState> : null}
        </TableScroll>
      </Card>
      {formVisible ? (
        <Card>
          <CardHeader icon="user" title={editing ? t("agents.edit") : t("agents.add")} />
          <div style={{ padding: 14, display: "flex", flexDirection: "column", gap: 10 }}>
            <label style={{ fontSize: 12, color: "var(--text-2)" }}>
              {t("agents.key")}
              <input
                className="z-field"
                style={{ display: "block", width: "100%", marginTop: 4 }}
                placeholder={t("agents.placeholder.key")}
                value={form.key}
                disabled={editing || busy}
                autoComplete="off"
                onChange={(e) => setForm((f) => ({ ...f, key: e.target.value }))}
              />
            </label>
            <label style={{ fontSize: 12, color: "var(--text-2)" }}>
              {t("agents.name")}
              <input
                className="z-field"
                style={{ display: "block", width: "100%", marginTop: 4 }}
                placeholder={t("agents.placeholder.name")}
                value={form.name}
                disabled={editing || busy}
                autoComplete="off"
                onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
              />
            </label>
            <label style={{ fontSize: 12, color: "var(--text-2)" }}>
              {t("agents.provider")}
              <select
                className="z-field"
                aria-label={t("agents.col.provider")}
                style={{ display: "block", width: "100%", marginTop: 4 }}
                value={form.llm_provider}
                disabled={busy}
                onChange={(e) => setForm((f) => ({ ...f, llm_provider: e.target.value }))}
              >
                <option value="">{t("agents.provider.default")}</option>
                {form.llm_provider && !providers.some((p) => p.name === form.llm_provider) ? (
                  <option value={form.llm_provider}>{form.llm_provider}</option>
                ) : null}
                {providers.map((p) => (
                  <option key={p.name} value={p.name}>
                    {p.name}
                  </option>
                ))}
              </select>
              <span style={{ display: "block", marginTop: 4, color: "var(--text-3)", fontSize: 11.5 }}>{t("agents.providerHint")}</span>
            </label>
            <label style={{ fontSize: 12, color: "var(--text-2)" }}>
              {t("agents.model")}
              <input
                className="z-field"
                style={{ display: "block", width: "100%", marginTop: 4 }}
                placeholder={t("agents.placeholder.model")}
                value={form.model}
                disabled={busy}
                autoComplete="off"
                onChange={(e) => setForm((f) => ({ ...f, model: e.target.value }))}
              />
              <span style={{ display: "block", marginTop: 4, color: "var(--text-3)", fontSize: 11.5 }}>{t("agents.modelHint")}</span>
            </label>
            <label style={{ fontSize: 12, color: "var(--text-2)" }}>
              {t("agents.instructions")}
              <textarea
                className="z-field"
                rows={6}
                style={{ display: "block", width: "100%", marginTop: 4, minHeight: 90, resize: "vertical" }}
                placeholder={t("agents.placeholder.instructions")}
                value={form.instructions}
                disabled={busy}
                onChange={(e) => setForm((f) => ({ ...f, instructions: e.target.value }))}
              />
            </label>
            <label style={{ fontSize: 12, color: "var(--text-2)" }}>
              {t("agents.mode")}
              <select
                className="z-field"
                aria-label={t("agents.col.mode")}
                style={{ display: "block", width: "100%", marginTop: 4 }}
                value={form.mode}
                disabled={busy}
                onChange={(e) => setForm((f) => ({ ...f, mode: e.target.value }))}
              >
                {form.mode ? null : <option value="">{t("agents.mode.unset")}</option>}
                {ORCHESTRATION_MODES.map((m) => (
                  <option key={m} value={m}>
                    {t(modeLabelKey(m))}
                  </option>
                ))}
              </select>
              <span style={{ display: "block", marginTop: 4, color: "var(--text-3)", fontSize: 11.5 }}>{t("agents.orchestrationHint")}</span>
              <span style={{ display: "block", marginTop: 4, color: "var(--text-3)", fontSize: 11.5 }}>{t("agents.promptHint")}</span>
            </label>
            <label style={{ fontSize: 12, color: "var(--text-2)", display: "flex", alignItems: "center", gap: 8 }}>
              <input
                type="checkbox"
                checked={form.enabled}
                disabled={busy}
                onChange={(e) => setForm((f) => ({ ...f, enabled: e.target.checked }))}
              />
              {t("agents.enabled")}
            </label>
            <div style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "center" }}>
              <Button variant="primary" disabled={busy} onClick={() => void save()}>
                {editing ? t("common.save") : t("agents.create")}
              </Button>
              <Button disabled={busy} onClick={resetForm}>
                {t("agents.cancelCreate")}
              </Button>
              {editing ? (
                <Button disabled={busy} onClick={() => void remove()} aria-label={t("agents.deleteNamed", { name: form.name || form.key || selectedId })}>
                  {t("common.delete")}
                </Button>
              ) : null}
            </div>
            {saving || deleting ? <StatusLine kind="loading" /> : null}
          </div>
        </Card>
      ) : null}
    </PageChrome>
  );
}
