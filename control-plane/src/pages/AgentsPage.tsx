import { useEffect, useState } from "react";
import { api, ORCHESTRATION_MODES, type Agent } from "../api/client";
import { useI18n, type MsgKey } from "../i18n";
import { Button } from "../ui/Button";
import { Card, CardHeader } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { SectionHeader } from "../ui/SectionHeader";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

function modeLabelKey(mode: string): MsgKey {
  if (mode === "auto") return "agents.mode.auto";
  if (mode === "explicit") return "agents.mode.explicit";
  if (mode === "manual") return "agents.mode.manual";
  return "agents.mode.unset";
}

export function AgentsPage() {
  const { t } = useI18n();
  const [agents, setAgents] = useState<Agent[]>([]);
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(true);
  const [key, setKey] = useState("");
  const [name, setName] = useState("");

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

  async function create() {
    if (!key.trim()) return;
    try {
      await api.createAgent({ agent_key: key.trim(), display_name: name.trim() || key.trim() });
      setKey("");
      setName("");
      await load();
    } catch (e) {
      setErr(formatPublicError(e));
    }
  }

  async function patchMode(id: string, mode: string) {
    if (!ORCHESTRATION_MODES.includes(mode as (typeof ORCHESTRATION_MODES)[number])) return;
    setLoading(true);
    try {
      await api.updateAgent(id, { orchestration_mode: mode });
      await load();
    } catch (e) {
      setErr(formatPublicError(e));
      setLoading(false);
    }
  }

  return (
    <div style={{ padding: "14px 22px 40px", display: "flex", flexDirection: "column", gap: 14 }}>
      <SectionHeader
        icon="bolt"
        title={t("agents.title")}
        description={t("agents.desc")}
        actions={
          <>
            <Button icon="refresh" iconGesture onClick={() => void load()}>
              {t("common.refresh")}
            </Button>
            <Button variant="primary" icon="plus" onClick={() => void create()}>
              {t("agents.create")}
            </Button>
          </>
        }
      />
      {err ? <StatusLine kind="error">{err}</StatusLine> : null}
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
        <input className="z-field" placeholder="agent_key" value={key} onChange={(e) => setKey(e.target.value)} />
        <input className="z-field" placeholder="display_name" value={name} onChange={(e) => setName(e.target.value)} />
      </div>
      <Card>
        <CardHeader icon="user" title={t("agents.list")} meta={t("agents.meta", { n: agents.length })} />
        <div style={{ display: "flex", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)" }}>
          <span style={{ flex: 1.4 }}>{t("agents.col.key")}</span>
          <span style={{ flex: 2 }}>{t("agents.col.name")}</span>
          <span style={{ flex: 2 }}>{t("agents.col.id")}</span>
          <span style={{ flex: 1.2 }}>{t("agents.col.model")}</span>
          <span style={{ flex: 1.4 }}>{t("agents.col.mode")}</span>
        </div>
        {agents.map((a) => (
          <div
            key={a.id}
            style={{ display: "flex", alignItems: "center", padding: "11px 16px", fontSize: 12.5, borderBottom: "1px solid var(--border-soft)" }}
          >
            <span style={{ flex: 1.4, fontWeight: 600 }}>{a.agent_key}</span>
            <span style={{ flex: 2 }}>{a.display_name}</span>
            <span style={{ flex: 2, color: "var(--text-3)", fontVariantNumeric: "tabular-nums" }}>{a.id}</span>
            <span style={{ flex: 1.2, color: "var(--text-2)" }}>{a.model || "—"}</span>
            <span style={{ flex: 1.4 }}>
              <select
                className="z-field"
                aria-label={t("agents.col.mode")}
                value={a.orchestration_mode || ""}
                disabled={loading}
                onChange={(e) => void patchMode(a.id, e.target.value)}
              >
                {a.orchestration_mode ? null : <option value="">{t("agents.mode.unset")}</option>}
                {ORCHESTRATION_MODES.map((m) => (
                  <option key={m} value={m}>
                    {t(modeLabelKey(m))}
                  </option>
                ))}
              </select>
            </span>
          </div>
        ))}
        {loading ? <StatusLine kind="loading" /> : agents.length === 0 ? <EmptyState>{t("agents.empty")}</EmptyState> : null}
      </Card>
    </div>
  );
}
