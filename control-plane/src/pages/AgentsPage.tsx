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
import { providersApi, type ProviderInfo } from "../api/providers";
import { useI18n, type MsgKey } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { SectionHeader } from "../ui/SectionHeader";
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
  const [err, setErr] = useState("");
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

  const editing = Boolean(selectedId);
  const busy = saving || deleting || loadingDetail;

  const visible = useMemo(
    () => filterAgents(agents, { query, status: statusFilter, provider: providerFilter }),
    [agents, query, statusFilter, providerFilter],
  );
  const providerOptions = useMemo(() => uniqueProviders(agents), [agents]);

  async function load() {
    setLoading(true);
    try {
      const [j, p] = await Promise.allSettled([api.listAgents(), providersApi.list()]);
      if (j.status === "fulfilled") {
        setAgents(j.value.agents ?? []);
      }
      if (p.status === "fulfilled") {
        setProviders(p.value.providers ?? []);
      }
      const fail = j.status === "rejected" ? j.reason : null;
      setErr(fail ? formatPublicError(fail) : "");
    } catch (e) {
      setErr(formatPublicError(e));
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
  }

  async function pick(a: Agent) {
    if (busy) return;
    setErr("");
    setLoadingDetail(true);
    try {
      const detail = await api.getAgent(a.id);
      applyDetail(detail);
    } catch (e) {
      setErr(mapErr(e));
    } finally {
      setLoadingDetail(false);
    }
  }

  function resetForm() {
    setSelectedId("");
    setUpdatedAt("");
    setForm(emptyForm);
    setErr("");
  }

  function mapErr(e: unknown): string {
    const kind = agentConflictKind(e);
    if (kind === "lead") return t("agents.cannotDeleteLead");
    if (kind === "inactive") return t("agents.inactive");
    if (kind === "exists") return t("agents.keyExists");
    if (kind === "conflict") return t("agents.conflict");
    return formatPublicError(e);
  }

  async function save() {
    setErr("");
    const keyErr = validateAgentKey(form.key, editing);
    if (keyErr) {
      setErr(t(keyErr));
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
        setErr(mapErr(e));
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
      setErr(mapErr(e));
    } finally {
      setSaving(false);
    }
  }

  async function remove() {
    if (!editing || busy) return;
    const current = agents.find((a) => a.id === selectedId);
    const named = agentDisplayName(current || { id: selectedId, display_name: form.name, agent_key: form.key });
    if (!window.confirm(t("agents.confirmDelete", { name: named }))) return;
    setDeleting(true);
    try {
      await api.deleteAgent(selectedId);
      resetForm();
      await load();
    } catch (e) {
      setErr(mapErr(e));
    } finally {
      setDeleting(false);
    }
  }

  const filteredEmpty = !loading && agents.length > 0 && visible.length === 0;

  return (
    <div style={{ padding: "14px 22px 40px", display: "flex", flexDirection: "column", gap: 14 }}>
      <SectionHeader
        icon="bolt"
        title={t("agents.title")}
        description={t("agents.desc")}
        actions={
          <Button icon="refresh" iconGesture onClick={() => void load()}>
            {t("common.refresh")}
          </Button>
        }
      />
      {err ? <StatusLine kind="error">{err}</StatusLine> : null}
      <Card>
        <CardHeader icon="user" title={t("agents.list")} meta={t("agents.meta", { n: visible.length })} />
        <div style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "center", padding: "10px 16px 8px" }}>
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
        </div>
        <TableScroll>
        <div style={{ display: "flex", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)", gap: 8 }}>
          <span style={{ flex: 1.4 }}>{t("agents.col.key")}</span>
          <span style={{ flex: 2 }}>{t("agents.col.name")}</span>
          <span style={{ flex: 1.1 }}>{t("agents.col.status")}</span>
          <span style={{ flex: 1.2 }}>{t("agents.col.provider")}</span>
          <span style={{ flex: 1.2 }}>{t("agents.col.model")}</span>
          <span style={{ flex: 1.4 }}>{t("agents.col.mode")}</span>
        </div>
        {visible.map((a) => {
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
        })}
        {loading || loadingDetail ? <StatusLine kind="loading" /> : null}
        {!loading && agents.length === 0 ? <EmptyState>{t("agents.empty")}</EmptyState> : null}
        {filteredEmpty ? <EmptyState>{t("agents.emptyFilter")}</EmptyState> : null}
        </TableScroll>
      </Card>
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
              {t("agents.new")}
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
    </div>
  );
}
