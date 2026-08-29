import { useEffect, useMemo, useState, type CSSProperties } from "react";
import { api } from "../api/client";
import {
  canClearBox,
  channelRemediation,
  channelsApi,
  DM_POLICIES,
  filterChannels,
  formatAllowFrom,
  GROUP_POLICIES,
  isPhase2,
  normalizeChannelRow,
  parseAllowFrom,
  sanitizePairingItem,
  secretPutBody,
  type ChannelHealthFilter,
  type ChannelPairingItem,
  type ChannelRemediation,
  type ChannelRow,
} from "../api/channels";
import { redactPublicText } from "../api/public-error";
import { useI18n, type MsgKey } from "../i18n";
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

function policyLabel(v: string): MsgKey {
  if (v === "open") return "channels.policy.open";
  if (v === "pairing") return "channels.policy.pairing";
  if (v === "allowlist") return "channels.policy.allowlist";
  return "channels.policy.disabled";
}

function remediateKey(kind: ChannelRemediation): MsgKey {
  if (kind === "missing") return "channels.remediate.missing";
  if (kind === "failed") return "channels.remediate.failed";
  if (kind === "parked") return "channels.remediate.parked";
  if (kind === "stopped") return "channels.remediate.stopped";
  if (kind === "from_env") return "channels.remediate.from_env";
  return "channels.remediate.ok";
}

type Draft = { bot_token: string; access_token: string; app_secret: string };

const emptyDraft = (): Draft => ({ bot_token: "", access_token: "", app_secret: "" });

const HEALTH_FILTERS: ChannelHealthFilter[] = ["running", "failed", "missing", "parked", "stopped"];

const pwStyle: CSSProperties = {
  fontSize: 12.5,
  padding: "6px 10px",
  borderRadius: 8,
  border: "1px solid var(--border)",
  background: "var(--card)",
  color: "var(--text)",
  width: "100%",
  boxSizing: "border-box",
};

export function ChannelsPage() {
  const { t } = useI18n();
  const [rows, setRows] = useState<ChannelRow[]>([]);
  const [lite, setLite] = useState(false);
  const [err, setErr] = useState("");
  const [ok, setOk] = useState("");
  const [loading, setLoading] = useState(true);
  const [copied, setCopied] = useState("");
  const [agents, setAgents] = useState<{ id: string; display_name?: string; agent_key?: string }[]>([]);
  const [pending, setPending] = useState<ChannelPairingItem[]>([]);
  const [qrStatus, setQrStatus] = useState("");
  const [drafts, setDrafts] = useState<Record<string, Draft>>({});
  const [allowDrafts, setAllowDrafts] = useState<Record<string, string>>({});
  const [saving, setSaving] = useState("");
  const [testing, setTesting] = useState("");
  const [clearing, setClearing] = useState("");
  const [query, setQuery] = useState("");
  const [healthFilter, setHealthFilter] = useState<ChannelHealthFilter>("");

  const visible = useMemo(() => filterChannels(rows, { query, health: healthFilter }), [rows, query, healthFilter]);
  const filteredEmpty = !loading && rows.length > 0 && visible.length === 0;

  async function load() {
    try {
      const j = await channelsApi.list();
      const list = (j.channels ?? []).map((c) => normalizeChannelRow(c)).filter((c): c is ChannelRow => c != null);
      setRows(list);
      setLite(j.lite === true);
      const ag = await api.listAgents().catch(() => ({ agents: [] }));
      setAgents(ag.agents ?? []);
      const pr = await channelsApi.pairingList().catch(() => ({ items: [] as ChannelPairingItem[] }));
      setPending(
        (pr.items ?? [])
          .map((i) => sanitizePairingItem(i))
          .filter((i) => i.status === "pending" && i.id),
      );
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

  async function patchPolicy(name: string, body: Record<string, unknown>) {
    setErr("");
    setOk("");
    try {
      await channelsApi.patch(name, body);
      setOk(t("channels.policySaved"));
      await load();
    } catch (e) {
      setErr(formatPublicError(e));
    }
  }

  const telegram = rows.find((c) => c.name === "telegram");
  const zaloOa = rows.find((c) => c.name === "zalo-oa");
  const telegramPolicy = telegram?.dm_policy || "";

  function draftOf(name: string): Draft {
    return drafts[name] ?? emptyDraft();
  }

  function setDraft(name: string, patch: Partial<Draft>) {
    setDrafts((prev) => ({ ...prev, [name]: { ...emptyDraft(), ...prev[name], ...patch } }));
  }

  function allowOf(c: ChannelRow): string {
    return allowDrafts[c.name] ?? formatAllowFrom(c.allow_from);
  }

  async function saveSecrets(c: ChannelRow) {
    const body = secretPutBody(c.writable, draftOf(c.name));
    if (!Object.keys(body).length) {
      setErr(t("channels.needSecret"));
      return;
    }
    setSaving(c.name);
    setErr("");
    setOk("");
    try {
      await channelsApi.putSecrets(c.name, body);
      setDrafts((prev) => ({ ...prev, [c.name]: emptyDraft() }));
      setOk(t("channels.secretSet"));
      await load();
    } catch (e) {
      const msg = formatPublicError(e);
      setErr(msg.includes("master key") ? t("channels.masterKey") : msg);
    } finally {
      setSaving("");
    }
  }

  async function clearSecrets(c: ChannelRow) {
    if (!canClearBox(c)) return;
    if (!window.confirm(t("channels.confirmClear", { name: c.name }))) return;
    setClearing(c.name);
    setErr("");
    setOk("");
    try {
      const r = await channelsApi.clearSecrets(c.name);
      setDrafts((prev) => ({ ...prev, [c.name]: emptyDraft() }));
      setOk(r.from_env ? t("channels.fromEnvClearHint") : t("channels.cleared"));
      await load();
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setClearing("");
    }
  }

  async function testChannel(name: string) {
    setTesting(name);
    setErr("");
    setOk("");
    try {
      const r = await channelsApi.test(name);
      setOk(t("channels.testOk", { health: r.health || "ok" }));
      await load();
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setTesting("");
    }
  }

  async function saveAllow(c: ChannelRow) {
    await patchPolicy(c.name, { allow_from: parseAllowFrom(allowOf(c)) });
    setAllowDrafts((prev) => {
      const next = { ...prev };
      delete next[c.name];
      return next;
    });
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
      {ok ? <p role="status" style={{ margin: 0, fontSize: 12.5, color: "var(--text-2)" }}>{ok}</p> : null}
      <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("channels.envOnly")}</p>
      <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("channels.noSecrets")}</p>
      <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("channels.emptyPut")}</p>
      {loading ? (
        <StatusLine kind="loading" />
      ) : lite ? (
        <p style={{ margin: 0, fontSize: 12, color: "var(--text-3)", lineHeight: 1.5 }}>{t("channels.liteOff")}</p>
      ) : (
        <>
        <Card>
          <CardHeader
            icon="hook"
            title={t("channels.pairing")}
            meta={pending.length ? t("channels.pairing.pending", { n: pending.length }) : "0"}
          />
          <div style={{ padding: "0 16px 16px", display: "flex", flexDirection: "column", gap: 8 }}>
            <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-2)", lineHeight: 1.45 }}>{t("channels.pairing.guide")}</p>
            <p style={{ margin: 0, fontSize: 12, color: "var(--text-3)" }}>{t("channels.pairing.noCode")}</p>
            {telegram ? (
              <label style={{ display: "flex", flexDirection: "column", gap: 4, maxWidth: 280 }}>
                <span style={{ fontSize: 11, fontWeight: 600 }}>{t("channels.pairing.dm")}</span>
                <select
                  className="z-field"
                  aria-label={t("channels.pairing.dm")}
                  value={telegram.dm_policy || ""}
                  onChange={(e) => void patchPolicy("telegram", { dm_policy: e.target.value })}
                >
                  {DM_POLICIES.map((p) => (
                    <option key={p} value={p}>{t(policyLabel(p))}</option>
                  ))}
                </select>
              </label>
            ) : null}
            {telegramPolicy === "open" ? (
              <>
                <p style={{ margin: 0, fontSize: 12, color: "var(--text-3)" }}>{t("channels.pairing.openHint")}</p>
                <div>
                  <Button onClick={() => void patchPolicy("telegram", { dm_policy: "pairing" })}>{t("channels.pairing.enable")}</Button>
                </div>
              </>
            ) : null}
            {zaloOa ? (
              <label style={{ display: "flex", flexDirection: "column", gap: 4, maxWidth: 280 }}>
                <span style={{ fontSize: 11, fontWeight: 600 }}>{t("channels.pairing.oaDm")}</span>
                <select
                  className="z-field"
                  aria-label={t("channels.pairing.oaDm")}
                  value={zaloOa.dm_policy || ""}
                  onChange={(e) => void patchPolicy("zalo-oa", { dm_policy: e.target.value })}
                >
                  {DM_POLICIES.map((p) => (
                    <option key={p} value={p}>{t(policyLabel(p))}</option>
                  ))}
                </select>
              </label>
            ) : null}
            {pending.length === 0 ? (
              <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("channels.pairing.empty")}</p>
            ) : (
              pending.map((p) => (
                <div
                  key={p.id}
                  style={{
                    display: "flex",
                    gap: 10,
                    alignItems: "center",
                    flexWrap: "wrap",
                    fontSize: 12.5,
                    padding: "8px 0",
                    borderTop: "1px solid var(--border-soft)",
                  }}
                >
                  <strong>{p.channel}</strong>
                  <span>{t("channels.pairing.sender", { id: p.sender_id || "—" })}</span>
                  {p.expires_at ? <span style={{ color: "var(--text-3)" }}>{t("channels.pairing.expires", { at: p.expires_at })}</span> : null}
                  <Button onClick={() => void channelsApi.pairingApprove(p.id).then(load).catch((e) => setErr(formatPublicError(e)))}>
                    {t("channels.pairing.approve")}
                  </Button>
                  <Button onClick={() => void channelsApi.pairingDeny(p.id).then(load).catch((e) => setErr(formatPublicError(e)))}>
                    {t("channels.pairing.deny")}
                  </Button>
                </div>
              ))
            )}
          </div>
        </Card>
        <Card>
          <CardHeader icon="hook" title={t("channels.list")} meta={t("channels.meta", { n: visible.length })} />
          <div style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "center", padding: "10px 16px 8px" }}>
            <input
              className="z-field"
              style={{ flex: 1, minWidth: 160 }}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder={t("channels.search")}
              aria-label={t("channels.search")}
              autoComplete="off"
            />
            <select
              className="z-field"
              aria-label={t("channels.filterHealth")}
              value={healthFilter}
              onChange={(e) => setHealthFilter(e.target.value as ChannelHealthFilter)}
              style={{ minWidth: 140 }}
            >
              <option value="">{t("channels.filterHealthAll")}</option>
              {HEALTH_FILTERS.map((h) => (
                <option key={h} value={h}>{t(`channels.health.${h}` as MsgKey)}</option>
              ))}
            </select>
          </div>
          <TableScroll>
          <div style={{ display: "flex", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)" }}>
            <span style={{ flex: 1.2 }}>{t("channels.col.name")}</span>
            <span style={{ flex: 1 }}>{t("channels.col.health")}</span>
            <span style={{ flex: 1.2 }}>{t("channels.col.policy")}</span>
            <span style={{ flex: 2.2 }}>{t("channels.col.envNames")}</span>
          </div>
          {visible.map((c) => {
            const parked = isPhase2(c);
            const writable = c.writable ?? [];
            const rem = channelRemediation(c);
            return (
            <div key={c.name} style={{ padding: "11px 16px", fontSize: 12.5, borderBottom: "1px solid var(--border-soft)" }}>
              <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                <span style={{ flex: 1.2, fontWeight: 600 }}>{c.name}</span>
                <span style={{ flex: 1, display: "flex", flexWrap: "wrap", gap: 6 }}>
                  {c.health ? <Badge tone={healthTone(c.health)}>{c.health}</Badge> : null}
                  {c.missing ? <Badge tone="warning">{t("channels.missing")}</Badge> : null}
                  {c.secret_set ? <Badge tone="positive">{t("channels.secretSet")}</Badge> : null}
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
                </span>
              </div>
              <p style={{ margin: "8px 0 0", fontSize: 12, color: "var(--text-3)" }}>{t(remediateKey(rem))}</p>
              {c.last_error ? <p style={{ margin: "4px 0 0", fontSize: 11, color: "var(--text-3)" }}>{redactPublicText(c.last_error)}</p> : null}
              {c.name === "zalo-personal" ? (
                <p style={{ margin: "8px 0 0", fontSize: 12, color: "var(--text-3)" }}>{t("channels.personalNoToken")}</p>
              ) : null}
              {parked ? (
                <p style={{ margin: "8px 0 0", fontSize: 12, color: "var(--text-3)" }}>{t("channels.parkedNoSecret")}</p>
              ) : (
                <div style={{ marginTop: 10, display: "flex", flexWrap: "wrap", gap: 10, alignItems: "flex-end" }}>
                  <label style={{ display: "flex", flexDirection: "column", gap: 4, minWidth: 140 }}>
                    <span style={{ fontSize: 11, fontWeight: 600 }}>{t("channels.dmPolicy")}</span>
                    <select
                      className="z-field"
                      aria-label={t("channels.dmPolicy")}
                      value={c.dm_policy || ""}
                      onChange={(e) => void patchPolicy(c.name, { dm_policy: e.target.value })}
                    >
                      {DM_POLICIES.map((p) => (
                        <option key={p} value={p}>{t(policyLabel(p))}</option>
                      ))}
                    </select>
                  </label>
                  <label style={{ display: "flex", flexDirection: "column", gap: 4, minWidth: 140 }}>
                    <span style={{ fontSize: 11, fontWeight: 600 }}>{t("channels.groupPolicy")}</span>
                    <select
                      className="z-field"
                      aria-label={t("channels.groupPolicy")}
                      value={c.group_policy || ""}
                      onChange={(e) => void patchPolicy(c.name, { group_policy: e.target.value })}
                    >
                      {GROUP_POLICIES.map((p) => (
                        <option key={p} value={p}>{t(policyLabel(p))}</option>
                      ))}
                    </select>
                  </label>
                  <label style={{ display: "flex", alignItems: "center", gap: 6, fontSize: 12 }}>
                    <input
                      type="checkbox"
                      checked={c.require_mention === true}
                      onChange={(e) => void patchPolicy(c.name, { require_mention: e.target.checked })}
                    />
                    {t("channels.requireMention")}
                  </label>
                  <label style={{ display: "flex", flexDirection: "column", gap: 4, flex: 1, minWidth: 180 }}>
                    <span style={{ fontSize: 11, fontWeight: 600 }}>{t("channels.allowFrom")}</span>
                    <textarea
                      className="z-field"
                      aria-label={t("channels.allowFrom")}
                      rows={2}
                      value={allowOf(c)}
                      onChange={(e) => setAllowDrafts((prev) => ({ ...prev, [c.name]: e.target.value }))}
                      style={{ resize: "vertical", minHeight: 48 }}
                    />
                    <span style={{ fontSize: 11, color: "var(--text-3)" }}>{t("channels.allowFromHint")}</span>
                    <div>
                      <Button onClick={() => void saveAllow(c)}>{t("channels.savePolicy")}</Button>
                    </div>
                  </label>
                </div>
              )}
              {writable.length > 0 ? (
                <div style={{ marginTop: 10, display: "flex", flexDirection: "column", gap: 8, maxWidth: 520 }}>
                  {c.from_env ? <p style={{ margin: 0, fontSize: 12, color: "var(--text-3)" }}>{t("channels.fromEnv")}</p> : null}
                  {writable.includes("bot_token") ? (
                    <label style={{ display: "flex", flexDirection: "column", gap: 4 }}>
                      <span style={{ fontSize: 11, fontWeight: 600 }}>{t("channels.botToken")}</span>
                      <input
                        type="password"
                        autoComplete="off"
                        placeholder="123456:ABC-DEF…"
                        value={draftOf(c.name).bot_token}
                        onChange={(e) => setDraft(c.name, { bot_token: e.target.value })}
                        style={pwStyle}
                      />
                      <span style={{ fontSize: 11, color: "var(--text-3)" }}>{t("channels.botTokenHint")}</span>
                    </label>
                  ) : null}
                  {writable.includes("access_token") ? (
                    <label style={{ display: "flex", flexDirection: "column", gap: 4 }}>
                      <span style={{ fontSize: 11, fontWeight: 600 }}>{t("channels.accessToken")}</span>
                      <input
                        type="password"
                        autoComplete="off"
                        value={draftOf(c.name).access_token}
                        onChange={(e) => setDraft(c.name, { access_token: e.target.value })}
                        style={pwStyle}
                      />
                    </label>
                  ) : null}
                  {writable.includes("app_secret") ? (
                    <label style={{ display: "flex", flexDirection: "column", gap: 4 }}>
                      <span style={{ fontSize: 11, fontWeight: 600 }}>{t("channels.appSecret")}</span>
                      <input
                        type="password"
                        autoComplete="off"
                        value={draftOf(c.name).app_secret}
                        onChange={(e) => setDraft(c.name, { app_secret: e.target.value })}
                        style={pwStyle}
                      />
                      <span style={{ fontSize: 11, color: "var(--text-3)" }}>{t("channels.secretHint")}</span>
                    </label>
                  ) : null}
                  <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
                    <Button disabled={saving === c.name} onClick={() => void saveSecrets(c)}>
                      {c.secret_set ? t("channels.rotate") : t("channels.saveSecrets")}
                    </Button>
                    {canClearBox(c) ? (
                      <Button disabled={clearing === c.name} onClick={() => void clearSecrets(c)}>
                        {t("channels.clear")}
                      </Button>
                    ) : null}
                    <Button disabled={testing === c.name} onClick={() => void testChannel(c.name)}>
                      {t("channels.test")}
                    </Button>
                  </div>
                </div>
              ) : c.name === "zalo-personal" ? (
                <div style={{ marginTop: 10 }}>
                  <Button disabled={testing === c.name} onClick={() => void testChannel(c.name)}>
                    {t("channels.test")}
                  </Button>
                </div>
              ) : null}
            </div>
            );
          })}
          {filteredEmpty ? <EmptyState>{t("channels.filterEmpty")}</EmptyState> : null}
          {rows.length === 0 ? <EmptyState>{t("channels.empty")}</EmptyState> : null}
          </TableScroll>
        </Card>
        <Card>
          <CardHeader icon="hook" title={t("channels.qr")} />
          <p style={{ margin: "0 16px 8px", fontSize: 12.5, color: "var(--text-3)" }}>{t("channels.qr.risk")}</p>
          <p style={{ margin: "0 16px 8px", fontSize: 12.5 }}>{t("channels.qr.status", { status: qrStatus || "—" })}</p>
          <div style={{ padding: "0 16px 16px" }}>
            <Button
              onClick={() => {
                if (!window.confirm(t("channels.logoutConfirm"))) return;
                void channelsApi.logoutPersonal().then(load).catch((e) => setErr(formatPublicError(e)));
              }}
            >
              {t("channels.qr.logout")}
            </Button>
          </div>
        </Card>
        </>
      )}
    </div>
  );
}
