import { useEffect, useState } from "react";
import { api, ORCHESTRATION_MODES, type Agent } from "../api/client";
import { useI18n, type MsgKey } from "../i18n";
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

const emptyForm = { key: "", name: "", model: "", instructions: "", mode: "" };

function formFromAgent(a: Agent) {
  return {
    key: a.agent_key,
    name: a.display_name,
    model: a.model || "",
    instructions: a.instructions || "",
    mode: a.orchestration_mode || "",
  };
}

export function AgentsPage() {
  const { t } = useI18n();
  const [agents, setAgents] = useState<Agent[]>([]);
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [selectedId, setSelectedId] = useState("");
  const [form, setForm] = useState(emptyForm);

  const editing = Boolean(selectedId);

  async function load() {
    setLoading(true);
    try {
      const j = await api.listAgents();
      setAgents(j.agents ?? []);
      setErr("");
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  function pick(a: Agent) {
    setSelectedId(a.id);
    setForm(formFromAgent(a));
    setErr("");
  }

  function resetForm() {
    setSelectedId("");
    setForm(emptyForm);
    setErr("");
  }

  async function save() {
    setErr("");
    if (!editing) {
      if (!form.key.trim()) {
        setErr(t("agents.needKey"));
        return;
      }
      setSaving(true);
      try {
        const created = await api.createAgent({
          agent_key: form.key.trim(),
          display_name: form.name.trim() || form.key.trim(),
          model: form.model.trim() || undefined,
          instructions: form.instructions.trim() || undefined,
          orchestration_mode: form.mode || undefined,
        });
        setSelectedId(created.id);
        setForm(formFromAgent(created));
        await load();
      } catch (e) {
        setErr(formatPublicError(e));
      } finally {
        setSaving(false);
      }
      return;
    }

    const body: { model?: string; instructions?: string; orchestration_mode?: string } = {
      model: form.model.trim(),
      instructions: form.instructions.trim(),
    };
    if (ORCHESTRATION_MODES.includes(form.mode as (typeof ORCHESTRATION_MODES)[number])) {
      body.orchestration_mode = form.mode;
    }
    setSaving(true);
    try {
      const updated = await api.updateAgent(selectedId, body);
      setForm(formFromAgent(updated));
      await load();
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setSaving(false);
    }
  }

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
        <CardHeader icon="user" title={t("agents.list")} meta={t("agents.meta", { n: agents.length })} />
        <TableScroll>
        <div style={{ display: "flex", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)", gap: 8 }}>
          <span style={{ flex: 1.4 }}>{t("agents.col.key")}</span>
          <span style={{ flex: 2 }}>{t("agents.col.name")}</span>
          <span style={{ flex: 2 }}>{t("agents.col.id")}</span>
          <span style={{ flex: 1.2 }}>{t("agents.col.model")}</span>
          <span style={{ flex: 1.4 }}>{t("agents.col.mode")}</span>
        </div>
        {agents.map((a) => {
          const on = selectedId === a.id;
          return (
            <button
              key={a.id}
              type="button"
              onClick={() => {
                if (saving) return;
                pick(a);
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
                cursor: "pointer",
                alignItems: "center",
              }}
            >
              <span style={{ flex: 1.4, fontWeight: 600 }}>{a.agent_key}</span>
              <span style={{ flex: 2 }}>{a.display_name}</span>
              <span style={{ flex: 2, color: "var(--text-3)", fontVariantNumeric: "tabular-nums" }}>{a.id}</span>
              <span style={{ flex: 1.2, color: "var(--text-2)" }}>{a.model || "—"}</span>
              <span style={{ flex: 1.4 }}>{t(modeLabelKey(a.orchestration_mode || ""))}</span>
            </button>
          );
        })}
        {loading ? <StatusLine kind="loading" /> : agents.length === 0 ? <EmptyState>{t("agents.empty")}</EmptyState> : null}
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
              disabled={editing}
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
              disabled={editing}
              autoComplete="off"
              onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
            />
          </label>
          <label style={{ fontSize: 12, color: "var(--text-2)" }}>
            {t("agents.model")}
            <input
              className="z-field"
              style={{ display: "block", width: "100%", marginTop: 4 }}
              placeholder={t("agents.placeholder.model")}
              value={form.model}
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
              disabled={saving}
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
          <div style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "center" }}>
            <Button variant="primary" disabled={saving} onClick={() => void save()}>
              {editing ? t("common.save") : t("agents.create")}
            </Button>
            <Button disabled={saving} onClick={resetForm}>
              {t("agents.new")}
            </Button>
          </div>
          {saving ? <StatusLine kind="loading" /> : null}
        </div>
      </Card>
    </div>
  );
}
