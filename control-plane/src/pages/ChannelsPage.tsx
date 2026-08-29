import { useEffect, useState } from "react";
import { api } from "../api/client";
import { channelsApi, type ChannelPairingItem, type ChannelRow } from "../api/channels";
import { useI18n } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { SectionHeader } from "../ui/SectionHeader";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

function healthTone(h: string): "positive" | "warning" | "critical" | "neutral" {
  if (h === "running") return "positive";
  if (h === "failed") return "critical";
  if (h === "missing" || h === "parked") return "warning";
  return "neutral";
}

export function ChannelsPage() {
  const { t } = useI18n();
  const [rows, setRows] = useState<ChannelRow[]>([]);
  const [lite, setLite] = useState(false);
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(true);
  const [copied, setCopied] = useState("");
  const [agents, setAgents] = useState<{ id: string; display_name?: string; agent_key?: string }[]>([]);
  const [pending, setPending] = useState<ChannelPairingItem[]>([]);
  const [qrStatus, setQrStatus] = useState("");

  async function load() {
    try {
      const j = await channelsApi.list();
      const list = (j.channels ?? []).map((c) => {
        const envNames = Array.isArray(c?.env_names)
          ? c.env_names.filter((n): n is string => typeof n === "string" && n.length > 0)
          : [];
        const env = typeof c?.env === "string" ? c.env : "";
        return {
          name: typeof c?.name === "string" ? c.name : "",
          configured: c?.configured === true,
          missing: c?.missing === true || c?.configured !== true,
          env,
          env_names: envNames.length ? envNames : env ? [env] : [],
          health: typeof c?.health === "string" ? c.health : "",
          dm_policy: typeof c?.dm_policy === "string" ? c.dm_policy : "",
          group_policy: typeof c?.group_policy === "string" ? c.group_policy : "",
          bound_agent_id: typeof c?.bound_agent_id === "string" ? c.bound_agent_id : "",
          phase: typeof c?.phase === "number" ? c.phase : 0,
          last_error: typeof c?.last_error === "string" ? c.last_error : "",
        } satisfies ChannelRow;
      });
      setRows(list.filter((c) => c.name));
      setLite(j.lite === true);
      const ag = await api.listAgents().catch(() => ({ agents: [] }));
      setAgents(ag.agents ?? []);
      const pr = await channelsApi.pairingList().catch(() => ({ items: [] as ChannelPairingItem[] }));
      setPending((pr.items ?? []).filter((i) => i.status === "pending"));
      const qr = await channelsApi.qr().catch(() => ({ status: "" }));
      setQrStatus(qr.status ?? "");
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

  async function copyEnv(env: string) {
    if (!env) return;
    try {
      await navigator.clipboard.writeText(env);
      setCopied(env);
      setErr("");
    } catch (e) {
      setErr(formatPublicError(e));
    }
  }

  async function bind(name: string, agentId: string) {
    try {
      await channelsApi.patch(name, { agent_id: agentId });
      await load();
    } catch (e) {
      setErr(formatPublicError(e));
    }
  }

  return (
    <div style={{ padding: "14px 22px 40px", display: "flex", flexDirection: "column", gap: 14 }}>
      <SectionHeader
        icon="hook"
        title={t("channels.title")}
        description={t("channels.desc")}
        actions={
          <Button icon="refresh" iconGesture onClick={() => void load()}>
            {t("common.refresh")}
          </Button>
        }
      />
      {err ? <StatusLine kind="error">{err}</StatusLine> : null}
      <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("channels.envOnly")}</p>
      <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("channels.noSecrets")}</p>
      {loading ? (
        <StatusLine kind="loading" />
      ) : lite ? (
        <p style={{ margin: 0, fontSize: 12, color: "var(--text-3)", lineHeight: 1.5 }}>{t("channels.liteOff")}</p>
      ) : (
        <>
        <Card>
          <CardHeader icon="hook" title={t("channels.list")} meta={t("channels.meta", { n: rows.length })} />
          <TableScroll>
          <div style={{ display: "flex", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)" }}>
            <span style={{ flex: 1.2 }}>{t("channels.col.name")}</span>
            <span style={{ flex: 1 }}>{t("channels.col.health")}</span>
            <span style={{ flex: 1.2 }}>{t("channels.col.policy")}</span>
            <span style={{ flex: 2.2 }}>{t("channels.col.envNames")}</span>
          </div>
          {rows.map((c) => (
            <div key={c.name} style={{ display: "flex", alignItems: "center", padding: "11px 16px", fontSize: 12.5, borderBottom: "1px solid var(--border-soft)", gap: 8 }}>
              <span style={{ flex: 1.2, fontWeight: 600 }}>{c.name}</span>
              <span style={{ flex: 1, display: "flex", flexWrap: "wrap", gap: 6 }}>
                {c.health ? <Badge tone={healthTone(c.health)}>{c.health}</Badge> : null}
                {c.missing ? <Badge tone="warning">{t("channels.missing")}</Badge> : null}
              </span>
              <span style={{ flex: 1.2, fontSize: 11, color: "var(--text-3)" }}>
                {[c.dm_policy, c.group_policy].filter(Boolean).join(" / ") || "—"}
                <div>
                  <select
                    aria-label={t("channels.agent")}
                    value={c.bound_agent_id ?? ""}
                    onChange={(e) => void bind(c.name, e.target.value)}
                    style={{ fontSize: 12, maxWidth: "100%" }}
                  >
                    <option value="">{t("channels.agent")}</option>
                    {agents.map((a) => (
                      <option key={a.id} value={a.id}>{a.display_name || a.agent_key || a.id}</option>
                    ))}
                  </select>
                </div>
              </span>
              <span style={{ flex: 2.2, display: "flex", flexDirection: "column", gap: 6, minWidth: 0 }}>
                {c.env_names.map((env) => (
                  <span key={env} style={{ display: "flex", alignItems: "center", gap: 8, minWidth: 0 }}>
                    <code style={{ fontFamily: "var(--font-mono, ui-monospace)", fontSize: 12, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", minWidth: 0 }}>{env}</code>
                    <Button onClick={() => void copyEnv(env)} style={{ padding: "4px 10px" }}>
                      {copied === env ? t("channels.copied") : t("common.copy")}
                    </Button>
                  </span>
                ))}
                {c.last_error ? <span style={{ fontSize: 11, color: "var(--text-3)" }}>{c.last_error}</span> : null}
              </span>
            </div>
          ))}
          {rows.length === 0 ? <EmptyState>{t("channels.empty")}</EmptyState> : null}
          </TableScroll>
        </Card>
        <Card>
          <CardHeader icon="hook" title={t("channels.pairing")} meta={String(pending.length)} />
          {pending.length === 0 ? <EmptyState>{t("channels.pairing.empty")}</EmptyState> : pending.map((p) => (
            <div key={p.id} style={{ display: "flex", gap: 8, alignItems: "center", padding: "8px 16px", fontSize: 12.5 }}>
              <span>{p.channel} {p.sender_id}</span>
              <Button onClick={() => void channelsApi.pairingApprove(p.id).then(load).catch((e) => setErr(formatPublicError(e)))}>{t("channels.pairing.approve")}</Button>
              <Button onClick={() => void channelsApi.pairingDeny(p.id).then(load).catch((e) => setErr(formatPublicError(e)))}>{t("channels.pairing.deny")}</Button>
            </div>
          ))}
        </Card>
        <Card>
          <CardHeader icon="hook" title={t("channels.qr")} />
          <p style={{ margin: "0 16px 8px", fontSize: 12.5, color: "var(--text-3)" }}>{t("channels.qr.risk")}</p>
          <p style={{ margin: "0 16px 8px", fontSize: 12.5 }}>{t("channels.qr.status", { status: qrStatus || "—" })}</p>
          <div style={{ padding: "0 16px 16px" }}>
            <Button onClick={() => void channelsApi.logoutPersonal().then(load).catch((e) => setErr(formatPublicError(e)))}>{t("channels.qr.logout")}</Button>
          </div>
        </Card>
        </>
      )}
    </div>
  );
}
