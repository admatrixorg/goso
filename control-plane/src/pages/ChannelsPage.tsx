import { useEffect, useMemo, useState, type CSSProperties } from "react";
import { api } from "../api/client";
import { confirmNamed } from "../api/confirm";
import {
  canClearBox,
  channelRemediation,
  channelsApi,
  DM_POLICIES,
  emptySecretDraft,
  filterChannels,
  formatAllowFrom,
  GROUP_POLICIES,
  isPhase2,
  isSecretDraftEmpty,
  normalizeChannelRow,
  pairingConfirmMatch,
  pairingLabel,
  pairingListHasSecrets,
  parseAllowFrom,
  publicPairingList,
  resolveSettled,
  secretMetaKind,
  secretPutBody,
  type ChannelHealthFilter,
  type ChannelPairingItem,
  type ChannelRemediation,
  type ChannelRow,
  type SecretDraft,
} from "../api/channels";
import { classifyPageState, formatStaleAt, inventoryBlocksMutation, isFilteredEmpty, listMetaCount } from "../api/page-state";
import { redactPublicText } from "../api/public-error";
import { useI18n, type MsgKey } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { PageChrome } from "../ui/PageChrome";
import { PageStatus } from "../ui/PageStatus";
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

function secretMetaKey(kind: ReturnType<typeof secretMetaKind>): MsgKey {
  if (kind === "env") return "channels.secretMeta.env";
  if (kind === "set") return "channels.secretMeta.set";
  return "channels.secretMeta.unset";
}

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
  const { t, locale } = useI18n();
  const [rows, setRows] = useState<ChannelRow[]>([]);
  const [lite, setLite] = useState(false);
  const [loading, setLoading] = useState(true);
  const [loaded, setLoaded] = useState(false);
  const [loadedAt, setLoadedAt] = useState<string | null>(null);
  const [err, setErr] = useState<unknown>(null);
  const [pending, setPending] = useState<ChannelPairingItem[]>([]);
  const [pairingLoading, setPairingLoading] = useState(true);
  const [pairingLoaded, setPairingLoaded] = useState(false);
  const [pairingLoadedAt, setPairingLoadedAt] = useState<string | null>(null);
  const [pairingErr, setPairingErr] = useState<unknown>(null);
  const [agents, setAgents] = useState<{ id: string; display_name?: string; agent_key?: string }[]>([]);
  const [agentsErr, setAgentsErr] = useState<unknown>(null);
  const [qrStatus, setQrStatus] = useState("");
  const [qrErr, setQrErr] = useState<unknown>(null);
  const [actionErr, setActionErr] = useState("");
  const [ok, setOk] = useState("");
  const [copied, setCopied] = useState("");
  const [drafts, setDrafts] = useState<Record<string, SecretDraft>>({});
  const [allowDrafts, setAllowDrafts] = useState<Record<string, string>>({});
  const [saving, setSaving] = useState("");
  const [testing, setTesting] = useState("");
  const [clearing, setClearing] = useState("");
  const [busy, setBusy] = useState("");
  const [query, setQuery] = useState("");
  const [healthFilter, setHealthFilter] = useState<ChannelHealthFilter>("");
  const [denyRow, setDenyRow] = useState<ChannelPairingItem | null>(null);
  const [typed, setTyped] = useState("");

  const catalogState = classifyPageState({
    loading,
    loaded,
    error: err,
    itemCount: rows.length,
    keepStale: loaded && rows.length > 0,
  });
  const pairingState = classifyPageState({
    loading: pairingLoading,
    loaded: pairingLoaded,
    error: pairingErr,
    itemCount: pending.length,
    keepStale: pairingLoaded && pending.length > 0,
  });
  const catalogBlocked = inventoryBlocksMutation(catalogState.kind);
  const pairingBlocked = inventoryBlocksMutation(pairingState.kind);
  const visible = useMemo(() => filterChannels(rows, { query, health: healthFilter }), [rows, query, healthFilter]);
  const filteredEmpty = isFilteredEmpty(catalogState, rows.length, visible.length);
  const catalogMeta = listMetaCount(catalogState.kind, visible.length);
  const pairingMeta = listMetaCount(pairingState.kind, pending.length);
  const telegram = catalogState.showItems ? rows.find((c) => c.name === "telegram") : undefined;
  const zaloOa = catalogState.showItems ? rows.find((c) => c.name === "zalo-oa") : undefined;
  const telegramPolicy = telegram?.dm_policy || "";
  const denyMatched = denyRow ? pairingConfirmMatch(typed, denyRow) : false;

  async function load() {
    setLoading(true);
    setPairingLoading(true);
    const [catRes, pairRes, agRes, qrRes] = await Promise.allSettled([
      channelsApi.list(),
      channelsApi.pairingList(),
      api.listAgents(),
      channelsApi.qr(),
    ]);
    const cat = resolveSettled(catRes);
    if (cat.ok) {
      const list = (cat.value.channels ?? []).map((c) => normalizeChannelRow(c)).filter((c): c is ChannelRow => c != null);
      setRows(list);
      setLite(cat.value.lite === true);
      setLoaded(true);
      setLoadedAt(new Date().toISOString());
      setErr(null);
      setDrafts({});
    } else {
      setErr(cat.error);
    }
    const pair = resolveSettled(pairRes);
    if (pair.ok) {
      const items = publicPairingList(pair.value.items);
      setPending(items);
      setPairingLoaded(true);
      setPairingLoadedAt(new Date().toISOString());
      setPairingErr(null);
      setActionErr(pairingListHasSecrets(pair.value.items) ? t("channels.pairing.leak") : "");
    } else {
      setPairingErr(pair.error);
    }
    const ag = resolveSettled(agRes);
    if (ag.ok) {
      setAgents(ag.value.agents ?? []);
      setAgentsErr(null);
    } else {
      setAgentsErr(ag.error);
    }
    const qr = resolveSettled(qrRes);
    if (qr.ok) {
      setQrStatus(qr.value.status ?? "");
      setQrErr(null);
    } else {
      setQrErr(qr.error);
    }
    setLoading(false);
    setPairingLoading(false);
  }

  useEffect(() => {
    void load();
  }, []);

  function draftOf(name: string): SecretDraft {
    return drafts[name] ?? emptySecretDraft();
  }

  function setDraft(name: string, patch: Partial<SecretDraft>) {
    setDrafts((prev) => ({ ...prev, [name]: { ...emptySecretDraft(), ...prev[name], ...patch } }));
  }

  function allowOf(c: ChannelRow): string {
    return allowDrafts[c.name] ?? formatAllowFrom(c.allow_from);
  }

  async function copyEnv(env: string) {
    if (!env || catalogBlocked) return;
    try {
      await navigator.clipboard.writeText(env);
      setCopied(env);
      setActionErr("");
    } catch (e) {
      setActionErr(formatPublicError(e));
    }
  }

  async function bind(name: string, agentId: string) {
    if (catalogBlocked) return;
    try {
      await channelsApi.patch(name, { agent_id: agentId });
      await load();
    } catch (e) {
      setActionErr(formatPublicError(e));
    }
  }

  async function patchPolicy(name: string, body: Record<string, unknown>) {
    if (catalogBlocked) return;
    setActionErr("");
    setOk("");
    try {
      await channelsApi.patch(name, body);
      setOk(t("channels.policySaved"));
      await load();
    } catch (e) {
      setActionErr(formatPublicError(e));
    }
  }

  async function saveSecrets(c: ChannelRow) {
    if (catalogBlocked) return;
    const body = secretPutBody(c.writable, draftOf(c.name));
    if (isSecretDraftEmpty(c.writable, draftOf(c.name))) {
      setActionErr(t("channels.needSecret"));
      return;
    }
    setSaving(c.name);
    setActionErr("");
    setOk("");
    try {
      await channelsApi.putSecrets(c.name, body);
      setDrafts((prev) => ({ ...prev, [c.name]: emptySecretDraft() }));
      setOk(t("channels.secretSet"));
      await load();
    } catch (e) {
      const msg = formatPublicError(e);
      setActionErr(msg.includes("master key") ? t("channels.masterKey") : msg);
    } finally {
      setSaving("");
    }
  }

  async function clearSecrets(c: ChannelRow) {
    if (catalogBlocked || !canClearBox(c)) return;
    if (!confirmNamed(t("channels.confirmClear", { name: c.name }), (m) => window.confirm(m))) return;
    setClearing(c.name);
    setActionErr("");
    setOk("");
    try {
      const r = await channelsApi.clearSecrets(c.name);
      setDrafts((prev) => ({ ...prev, [c.name]: emptySecretDraft() }));
      setOk(r.from_env ? t("channels.fromEnvClearHint") : t("channels.cleared"));
      await load();
    } catch (e) {
      setActionErr(formatPublicError(e));
    } finally {
      setClearing("");
    }
  }

  async function testChannel(name: string) {
    if (catalogBlocked) return;
    setTesting(name);
    setActionErr("");
    setOk("");
    try {
      const r = await channelsApi.test(name);
      setOk(t("channels.testOk", { health: r.health || "ok" }));
      await load();
    } catch (e) {
      setActionErr(formatPublicError(e));
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

  async function approvePairing(p: ChannelPairingItem) {
    if (pairingBlocked) return;
    setBusy("pair:" + p.id);
    setActionErr("");
    setOk("");
    try {
      await channelsApi.pairingApprove(p.id);
      setOk(t("channels.pairing.approveOk"));
      await load();
    } catch (e) {
      setActionErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  async function submitDeny() {
    if (!denyRow || pairingBlocked) return;
    if (!pairingConfirmMatch(typed, denyRow)) {
      setActionErr(t("channels.pairing.mismatch"));
      return;
    }
    setBusy("pair:" + denyRow.id);
    try {
      await channelsApi.pairingDeny(denyRow.id);
      setOk(t("channels.pairing.denyOk"));
      setActionErr("");
      setDenyRow(null);
      setTyped("");
      await load();
    } catch (e) {
      setActionErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  async function logoutPersonal() {
    if (catalogBlocked) return;
    if (!confirmNamed(t("channels.logoutConfirm", { name: "zalo-personal" }), (m) => window.confirm(m))) return;
    setBusy("logout");
    setActionErr("");
    setOk("");
    try {
      await channelsApi.logoutPersonal();
      await load();
    } catch (e) {
      setActionErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  return (
    <PageChrome
      icon="hook"
      title={t("channels.title")}
      description={t("channels.desc")}
      primary={
        <Button icon="refresh" iconGesture variant="primary" onClick={() => void load()} disabled={loading || pairingLoading || Boolean(busy)}>
          {t("common.refresh")}
        </Button>
      }
      filters={
        catalogState.showItems || catalogState.showEmpty ? (
          <>
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
                <option key={h} value={h}>
                  {t(`channels.health.${h}` as MsgKey)}
                </option>
              ))}
            </select>
          </>
        ) : null
      }
    >
      <Card>
        <CardHeader icon="hook" title={t("channels.how")} />
        <p style={{ margin: 0, padding: "0 16px 8px", fontSize: 12.5, color: "var(--text-3)", maxWidth: 720 }}>{t("channels.howBody")}</p>
        <p style={{ margin: 0, padding: "0 16px 14px", fontSize: 12.5, color: "var(--text-3)", maxWidth: 720 }}>{t("channels.noCreate")}</p>
      </Card>
      <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("channels.envOnly")}</p>
      <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("channels.noSecrets")}</p>
      <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("channels.emptyPut")}</p>
      <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("channels.testValidation")}</p>
      <PageStatus
        kind={catalogState.kind}
        errorText={err ? formatPublicError(err) : ""}
        staleAt={formatStaleAt(loadedAt, locale)}
        onReload={() => void load()}
      />
      {actionErr ? <StatusLine kind="error">{actionErr}</StatusLine> : null}
      {ok && !actionErr ? (
        <p role="status" style={{ margin: 0, fontSize: 12.5, color: "var(--text-2)" }}>
          {ok}
        </p>
      ) : null}

      <Card>
        <CardHeader
          icon="hook"
          title={t("channels.pairing")}
          meta={pairingMeta == null ? "—" : pending.length ? t("channels.pairing.pending", { n: pairingMeta }) : t("channels.pairing.pending", { n: 0 })}
        />
        <div style={{ padding: "0 16px 16px", display: "flex", flexDirection: "column", gap: 8 }}>
          <PageStatus
            kind={pairingState.kind}
            errorText={pairingErr ? formatPublicError(pairingErr) : ""}
            staleAt={formatStaleAt(pairingLoadedAt, locale)}
            onReload={() => void load()}
          />
          <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-2)", lineHeight: 1.45 }}>{t("channels.pairing.guide")}</p>
          <p style={{ margin: 0, fontSize: 12, color: "var(--text-3)" }}>{t("channels.pairing.noCode")}</p>
          {telegram && !catalogBlocked ? (
            <label style={{ display: "flex", flexDirection: "column", gap: 4, maxWidth: 280 }}>
              <span style={{ fontSize: 11, fontWeight: 600 }}>{t("channels.pairing.dm")}</span>
              <select
                className="z-field"
                aria-label={t("channels.pairing.dm")}
                value={telegram.dm_policy || ""}
                onChange={(e) => void patchPolicy("telegram", { dm_policy: e.target.value })}
              >
                {DM_POLICIES.map((p) => (
                  <option key={p} value={p}>
                    {t(policyLabel(p))}
                  </option>
                ))}
              </select>
            </label>
          ) : null}
          {telegramPolicy === "open" && !catalogBlocked ? (
            <>
              <p style={{ margin: 0, fontSize: 12, color: "var(--text-3)" }}>{t("channels.pairing.openHint")}</p>
              <div>
                <Button onClick={() => void patchPolicy("telegram", { dm_policy: "pairing" })}>{t("channels.pairing.enable")}</Button>
              </div>
            </>
          ) : null}
          {zaloOa && !catalogBlocked ? (
            <label style={{ display: "flex", flexDirection: "column", gap: 4, maxWidth: 280 }}>
              <span style={{ fontSize: 11, fontWeight: 600 }}>{t("channels.pairing.oaDm")}</span>
              <select
                className="z-field"
                aria-label={t("channels.pairing.oaDm")}
                value={zaloOa.dm_policy || ""}
                onChange={(e) => void patchPolicy("zalo-oa", { dm_policy: e.target.value })}
              >
                {DM_POLICIES.map((p) => (
                  <option key={p} value={p}>
                    {t(policyLabel(p))}
                  </option>
                ))}
              </select>
            </label>
          ) : null}
          {denyRow && !pairingBlocked ? (
            <div style={{ display: "flex", flexDirection: "column", gap: 8, padding: "8px 0", borderTop: "1px solid var(--border-soft)" }}>
              <strong>{t("channels.pairing.confirmDenyTitle")}</strong>
              <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-2)" }}>
                {t("channels.pairing.confirmDenyPreview", { name: pairingLabel(denyRow) })}
              </p>
              <p style={{ margin: 0, fontSize: 12, color: "var(--text-3)" }}>{t("channels.pairing.confirmDenyHint")}</p>
              <input
                className="z-field"
                value={typed}
                onChange={(e) => setTyped(e.target.value)}
                placeholder={t("channels.pairing.confirmPlaceholder")}
                aria-label={t("channels.pairing.confirmPlaceholder")}
                autoComplete="off"
                spellCheck={false}
              />
              <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
                <Button
                  variant="primary"
                  disabled={!denyMatched || Boolean(busy)}
                  onClick={() => void submitDeny()}
                  style={{ background: "var(--red)", borderColor: "transparent" }}
                >
                  {t("channels.pairing.confirmDeny")}
                </Button>
                <Button
                  variant="quiet"
                  disabled={Boolean(busy)}
                  onClick={() => {
                    setDenyRow(null);
                    setTyped("");
                  }}
                >
                  {t("channels.pairing.cancel")}
                </Button>
              </div>
            </div>
          ) : null}
          {pairingState.showEmpty ? <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("channels.pairing.empty")}</p> : null}
          {pairingState.showItems
            ? pending.map((p) => {
                const rowBusy = busy === "pair:" + p.id;
                return (
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
                    <Button disabled={pairingBlocked || rowBusy} onClick={() => void approvePairing(p)}>
                      {t("channels.pairing.approve")}
                    </Button>
                    <Button
                      disabled={pairingBlocked || rowBusy}
                      onClick={() => {
                        setDenyRow(p);
                        setTyped("");
                        setOk("");
                        setActionErr("");
                      }}
                    >
                      {t("channels.pairing.deny")}
                    </Button>
                  </div>
                );
              })
            : null}
        </div>
      </Card>

      <Card>
        <CardHeader icon="hook" title={t("channels.list")} meta={catalogMeta == null ? "—" : t("channels.meta", { n: catalogMeta })} />
        {lite && catalogState.showEmpty ? (
          <p style={{ margin: 0, padding: "12px 16px", fontSize: 12, color: "var(--text-3)", lineHeight: 1.5 }}>{t("channels.liteOff")}</p>
        ) : null}
        {agentsErr && catalogState.showItems ? (
          <p style={{ margin: 0, padding: "8px 16px 0", fontSize: 12.5, color: "var(--text-3)" }}>{t("channels.agentsUnavailable")}</p>
        ) : null}
        <TableScroll>
          <div style={{ display: "flex", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)" }}>
            <span style={{ flex: 1.2 }}>{t("channels.col.name")}</span>
            <span style={{ flex: 1 }}>{t("channels.col.health")}</span>
            <span style={{ flex: 1.2 }}>{t("channels.col.policy")}</span>
            <span style={{ flex: 2.2 }}>{t("channels.col.envNames")}</span>
          </div>
          {catalogState.showItems
            ? visible.map((c) => {
                const parked = isPhase2(c);
                const writable = c.writable ?? [];
                const rem = channelRemediation(c);
                const meta = secretMetaKind(c);
                return (
                  <div key={c.name} style={{ padding: "11px 16px", fontSize: 12.5, borderBottom: "1px solid var(--border-soft)" }}>
                    <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                      <span style={{ flex: 1.2, fontWeight: 600 }}>{c.name}</span>
                      <span style={{ flex: 1, display: "flex", flexWrap: "wrap", gap: 6 }}>
                        <Badge tone={c.enabled ? "positive" : "neutral"}>{c.enabled ? t("channels.enabled") : t("channels.disabled")}</Badge>
                        {c.health ? <Badge tone={healthTone(c.health)}>{c.health}</Badge> : null}
                        {c.missing ? <Badge tone="warning">{t("channels.missing")}</Badge> : null}
                        <Badge tone={meta === "unset" ? "neutral" : "positive"}>{t(secretMetaKey(meta))}</Badge>
                      </span>
                      <span style={{ flex: 1.2, fontSize: 11, color: "var(--text-3)" }}>
                        {[c.dm_policy, c.group_policy].filter(Boolean).join(" / ") || "—"}
                        <div>
                          <select
                            aria-label={t("channels.agent")}
                            value={c.bound_agent_id ?? ""}
                            disabled={catalogBlocked || Boolean(agentsErr)}
                            onChange={(e) => void bind(c.name, e.target.value)}
                            style={{ fontSize: 12, maxWidth: "100%" }}
                          >
                            <option value="">{t("channels.agent")}</option>
                            {agents.map((a) => (
                              <option key={a.id} value={a.id}>
                                {a.display_name || a.agent_key || a.id}
                              </option>
                            ))}
                          </select>
                        </div>
                      </span>
                      <span style={{ flex: 2.2, display: "flex", flexDirection: "column", gap: 6, minWidth: 0 }}>
                        {c.env_names.map((env) => (
                          <span key={env} style={{ display: "flex", alignItems: "center", gap: 8, minWidth: 0 }}>
                            <code style={{ fontFamily: "var(--font-mono, ui-monospace)", fontSize: 12, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", minWidth: 0 }}>{env}</code>
                            <Button disabled={catalogBlocked} onClick={() => void copyEnv(env)} style={{ padding: "4px 10px" }}>
                              {copied === env ? t("channels.copied") : t("common.copy")}
                            </Button>
                          </span>
                        ))}
                      </span>
                    </div>
                    <p style={{ margin: "8px 0 0", fontSize: 12, color: "var(--text-3)" }}>
                      {t("channels.col.diagnosis")}: {c.last_error ? redactPublicText(c.last_error) : "—"}
                    </p>
                    <p style={{ margin: "4px 0 0", fontSize: 12, color: "var(--text-3)" }}>
                      {t("channels.col.next")}: {t(remediateKey(rem))}
                    </p>
                    {c.name === "zalo-personal" ? (
                      <p style={{ margin: "8px 0 0", fontSize: 12, color: "var(--text-3)" }}>{t("channels.personalNoToken")}</p>
                    ) : null}
                    {parked ? (
                      <p style={{ margin: "8px 0 0", fontSize: 12, color: "var(--text-3)" }}>{t("channels.parkedNoSecret")}</p>
                    ) : catalogBlocked ? null : (
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
                              <option key={p} value={p}>
                                {t(policyLabel(p))}
                              </option>
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
                              <option key={p} value={p}>
                                {t(policyLabel(p))}
                              </option>
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
                    {writable.length > 0 && !catalogBlocked ? (
                      <div style={{ marginTop: 10, display: "flex", flexDirection: "column", gap: 8, maxWidth: 520 }}>
                        {c.from_env ? <p style={{ margin: 0, fontSize: 12, color: "var(--text-3)" }}>{t("channels.fromEnv")}</p> : null}
                        {writable.includes("bot_token") ? (
                          <label style={{ display: "flex", flexDirection: "column", gap: 4 }}>
                            <span style={{ fontSize: 11, fontWeight: 600 }}>{t("channels.botToken")}</span>
                            <input
                              type="password"
                              autoComplete="off"
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
                    ) : c.name === "zalo-personal" && !catalogBlocked ? (
                      <div style={{ marginTop: 10 }}>
                        <Button disabled={testing === c.name} onClick={() => void testChannel(c.name)}>
                          {t("channels.test")}
                        </Button>
                      </div>
                    ) : null}
                  </div>
                );
              })
            : null}
          {filteredEmpty ? <EmptyState>{t("channels.filterEmpty")}</EmptyState> : null}
          {catalogState.showEmpty && !lite ? <EmptyState>{t("channels.empty")}</EmptyState> : null}
        </TableScroll>
      </Card>

      <Card>
        <CardHeader icon="hook" title={t("channels.qr")} />
        <p style={{ margin: "0 16px 8px", fontSize: 12.5, color: "var(--text-3)" }}>{t("channels.qr.risk")}</p>
        {qrErr ? (
          <p style={{ margin: "0 16px 8px", fontSize: 12.5, color: "var(--text-3)" }}>{t("channels.qr.unavailable")}</p>
        ) : (
          <p style={{ margin: "0 16px 8px", fontSize: 12.5 }}>{t("channels.qr.status", { status: qrStatus || "—" })}</p>
        )}
        <div style={{ padding: "0 16px 16px" }}>
          <Button disabled={catalogBlocked || busy === "logout"} onClick={() => void logoutPersonal()}>
            {t("channels.qr.logout")}
          </Button>
        </div>
      </Card>
    </PageChrome>
  );
}
