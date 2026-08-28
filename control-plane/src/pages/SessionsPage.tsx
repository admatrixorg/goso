import { forwardRef, useEffect, useImperativeHandle, useRef, useState } from "react";
import { api, type Agent, type Session } from "../api/client";
import { useI18n } from "../i18n";
import { Button } from "../ui/Button";
import { Card, CardHeader } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { SectionHeader } from "../ui/SectionHeader";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

export type SessionsPageHandle = {
  focusCreate: () => void;
};

export const SessionsPage = forwardRef<
  SessionsPageHandle,
  { onPick: (id: string, label?: string) => void; compact?: boolean; selectedId?: string }
>(function SessionsPage({ onPick, compact, selectedId }, ref) {
  const { t } = useI18n();
  const [sessions, setSessions] = useState<Session[]>([]);
  const [agents, setAgents] = useState<Agent[]>([]);
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [agentId, setAgentId] = useState("");
  const [label, setLabel] = useState("");
  const createBoxRef = useRef<HTMLDivElement>(null);
  const agentSelectRef = useRef<HTMLSelectElement>(null);

  useImperativeHandle(ref, () => ({
    focusCreate() {
      createBoxRef.current?.scrollIntoView({ behavior: "smooth", block: "nearest" });
      agentSelectRef.current?.focus();
    },
  }));

  async function load() {
    setLoading(true);
    const [sessRes, agRes] = await Promise.allSettled([api.listSessions(), api.listAgents()]);
    if (sessRes.status === "fulfilled") {
      setSessions(sessRes.value.sessions ?? []);
    }
    if (agRes.status === "fulfilled") {
      setAgents(agRes.value.agents ?? []);
    }
    const fail = sessRes.status === "rejected" ? sessRes.reason : agRes.status === "rejected" ? agRes.reason : null;
    setErr(fail ? formatPublicError(fail) : "");
    setLoading(false);
  }

  useEffect(() => {
    void load();
  }, []);

  async function create() {
    if (creating || loading) return;
    setErr("");
    if (agents.length === 0) {
      setErr(t("sessions.noAgents"));
      return;
    }
    const picked = agentId.trim();
    if (!picked) {
      setErr(t("sessions.needAgent"));
      return;
    }
    const trimmedLabel = label.trim();
    setCreating(true);
    try {
      const created = await api.createSession(trimmedLabel ? { agent_id: picked, label: trimmedLabel } : { agent_id: picked });
      setLabel("");
      setSessions((prev) => [created, ...prev.filter((s) => s.id !== created.id)]);
      onPick(created.id, created.label || created.id);
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setCreating(false);
    }
  }

  const noAgents = !loading && !err && agents.length === 0;

  function createFields() {
    return (
      <div ref={createBoxRef} style={{ display: "flex", flexDirection: "column", gap: compact ? 8 : 10 }}>
        {noAgents ? (
          <EmptyState style={{ padding: compact ? "10px 4px" : undefined }}>{t("sessions.noAgents")}</EmptyState>
        ) : null}
        <label style={{ fontSize: 12, color: "var(--text-2)" }}>
          {t("sessions.col.agent")}
          <select
            ref={agentSelectRef}
            className="z-field"
            aria-label={t("sessions.col.agent")}
            style={{ display: "block", width: "100%", marginTop: 4 }}
            value={agentId}
            disabled={creating || agents.length === 0}
            onChange={(e) => setAgentId(e.target.value)}
          >
            <option value="">{t("sessions.pickAgent")}</option>
            {agents.map((a) => (
              <option key={a.id} value={a.id}>
                {a.display_name || a.agent_key}
              </option>
            ))}
          </select>
        </label>
        <label style={{ fontSize: 12, color: "var(--text-2)" }}>
          {t("sessions.label")}
          <input
            className="z-field"
            style={{ display: "block", width: "100%", marginTop: 4 }}
            placeholder={t("sessions.placeholder.label")}
            value={label}
            disabled={creating}
            autoComplete="off"
            onChange={(e) => setLabel(e.target.value)}
          />
        </label>
        <Button variant="primary" disabled={creating || loading || agents.length === 0} onClick={() => void create()}>
          {t("sessions.create")}
        </Button>
      </div>
    );
  }

  if (compact) {
    return (
      <div style={{ padding: 12, display: "flex", flexDirection: "column", gap: 8 }}>
        <div style={{ display: "flex", alignItems: "center", gap: 8, padding: "4px 4px 8px" }}>
          <b style={{ fontSize: 13.5, fontWeight: 600, flex: 1 }}>{t("sessions.title")}</b>
          <Button icon="refresh" iconGesture variant="ghost" onClick={() => void load()} style={{ padding: "4px 8px" }}>
            {t("common.refresh")}
          </Button>
        </div>
        <div style={{ padding: "0 4px 4px" }}>{createFields()}</div>
        {err ? <StatusLine kind="error">{err}</StatusLine> : null}
        {loading || creating ? <StatusLine kind="loading" /> : null}
        {sessions.map((s) => {
          const selected = Boolean(selectedId) && selectedId === s.id;
          return (
            <button
              key={s.id}
              type="button"
              aria-current={selected ? "true" : undefined}
              onClick={() => onPick(s.id, s.label || s.id)}
              style={{
                display: "block",
                textAlign: "left",
                background: selected ? "var(--accent-soft)" : "var(--card)",
                border: `1px solid ${selected ? "var(--accent)" : "var(--border)"}`,
                borderRadius: 11,
                padding: "10px 12px",
                transition: "background var(--dur-hover) var(--ease-standard), border-color var(--dur-hover) var(--ease-standard)",
              }}
            >
              <div style={{ fontSize: 13, fontWeight: 600, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                {s.label || s.id}
              </div>
              <div style={{ fontSize: 11.5, color: "var(--text-3)", marginTop: 3 }}>{t("sessions.agent", { id: s.agent_id })}</div>
            </button>
          );
        })}
        {!loading && sessions.length === 0 ? <EmptyState style={{ padding: "16px 8px" }}>{t("sessions.empty")}</EmptyState> : null}
      </div>
    );
  }

  return (
    <div style={{ padding: "14px 22px 40px", display: "flex", flexDirection: "column", gap: 14 }}>
      <SectionHeader
        icon="list"
        title={t("sessions.title")}
        description={t("sessions.desc")}
        actions={
          <Button icon="refresh" iconGesture onClick={() => void load()}>
            {t("common.refresh")}
          </Button>
        }
      />
      {err ? <StatusLine kind="error">{err}</StatusLine> : null}
      <Card>
        <CardHeader icon="plus" title={t("sessions.add")} />
        <div style={{ padding: 14, display: "flex", flexDirection: "column", gap: 10 }}>
          {createFields()}
          {creating ? <StatusLine kind="loading" /> : null}
        </div>
      </Card>
      <Card>
        <CardHeader icon="msg" title={t("sessions.open")} meta={t("sessions.meta", { n: sessions.length })} />
        <div style={{ display: "flex", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)" }}>
          <span style={{ flex: 2.4 }}>{t("sessions.col.session")}</span>
          <span style={{ flex: 2 }}>{t("sessions.col.agent")}</span>
          <span style={{ flex: 1.2, textAlign: "right" }}></span>
        </div>
        {sessions.map((s) => (
          <div
            key={s.id}
            style={{
              display: "flex",
              alignItems: "center",
              padding: "11px 16px",
              fontSize: 12.5,
              borderBottom: "1px solid var(--border-soft)",
              cursor: "pointer",
            }}
            onClick={() => onPick(s.id, s.label || s.id)}
          >
            <span style={{ flex: 2.4, fontWeight: 600 }}>{s.label || s.id}</span>
            <span style={{ flex: 2, color: "var(--text-2)" }}>{s.agent_id}</span>
            <span style={{ flex: 1.2, textAlign: "right", color: "var(--accent)", fontWeight: 600, fontSize: 12 }}>{t("sessions.openChat")}</span>
          </div>
        ))}
        {loading ? <StatusLine kind="loading" /> : sessions.length === 0 ? <EmptyState>{t("sessions.empty")}</EmptyState> : null}
      </Card>
    </div>
  );
});
