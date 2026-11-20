import { useEffect, useMemo, useState } from "react";
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
import { Button } from "../ui/Button";
import { Card, CardHeader } from "../ui/Card";
import { DataTable, THead, Th, Td, Tr } from "../ui/DataTable";
import { EmptyState } from "../ui/EmptyState";
import { PageChrome } from "../ui/PageChrome";
import { PageStatus } from "../ui/PageStatus";
import { SearchInput } from "../ui/SearchInput";
import { StatusBadge, type Status } from "../ui/StatusBadge";
import { StatusLine, formatPublicError } from "../ui/StatusLine";
import { fieldClass, labelClass, selectClass } from "../ui/field";

function healthStatus(h: string): Status {
  if (h === "running") return "success";
  if (h === "failed") return "error";
  if (h === "missing" || h === "parked") return "warning";
  return "default";
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
      icon="radio"
      title={t("channels.title")}
      description={t("channels.desc")}
      refresh={
        <Button icon="refresh" iconGesture variant="outline" onClick={() => void load()} disabled={loading || pairingLoading || Boolean(busy)}>
          {t("common.refresh")}
        </Button>
      }
      filters={
        catalogState.showItems || catalogState.showEmpty ? (
          <>
            <SearchInput
              value={query}
              onChange={setQuery}
              placeholder={t("channels.search")}
              aria-label={t("channels.search")}
            />
            <select
              className={selectClass}
              aria-label={t("channels.filterHealth")}
              value={healthFilter}
              onChange={(e) => setHealthFilter(e.target.value as ChannelHealthFilter)}
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
        <p className="m-0 max-w-[720px] px-4 pb-2 text-sm text-muted-foreground">{t("channels.howBody")}</p>
        <p className="m-0 max-w-[720px] px-4 pb-4 text-sm text-muted-foreground">{t("channels.noCreate")}</p>
      </Card>
      <p className="m-0 text-sm text-muted-foreground">{t("channels.envOnly")}</p>
      <p className="m-0 text-sm text-muted-foreground">{t("channels.noSecrets")}</p>
      <p className="m-0 text-sm text-muted-foreground">{t("channels.emptyPut")}</p>
      <p className="m-0 text-sm text-muted-foreground">{t("channels.testValidation")}</p>
      <PageStatus
        kind={catalogState.kind}
        errorText={err ? formatPublicError(err) : ""}
        staleAt={formatStaleAt(loadedAt, locale)}
        onReload={() => void load()}
      />
      {actionErr ? <StatusLine kind="error">{actionErr}</StatusLine> : null}
      {ok && !actionErr ? (
        <p role="status" className="m-0 text-sm text-muted-foreground">
          {ok}
        </p>
      ) : null}

      <Card>
        <CardHeader
          icon="hook"
          title={t("channels.pairing")}
          meta={pairingMeta == null ? "—" : pending.length ? t("channels.pairing.pending", { n: pairingMeta }) : t("channels.pairing.pending", { n: 0 })}
        />
        <div className="flex flex-col gap-3 p-4">
          <PageStatus
            kind={pairingState.kind}
            errorText={pairingErr ? formatPublicError(pairingErr) : ""}
            staleAt={formatStaleAt(pairingLoadedAt, locale)}
            onReload={() => void load()}
          />
          <p className="m-0 text-sm text-muted-foreground leading-relaxed">{t("channels.pairing.guide")}</p>
          <p className="m-0 text-sm text-muted-foreground">{t("channels.pairing.noCode")}</p>
          {telegram && !catalogBlocked ? (
            <label className={`flex max-w-[280px] flex-col gap-1 ${labelClass}`}>
              {t("channels.pairing.dm")}
              <select
                className={selectClass}
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
              <p className="m-0 text-sm text-muted-foreground">{t("channels.pairing.openHint")}</p>
              <div>
                <Button variant="outline" onClick={() => void patchPolicy("telegram", { dm_policy: "pairing" })}>
                  {t("channels.pairing.enable")}
                </Button>
              </div>
            </>
          ) : null}
          {zaloOa && !catalogBlocked ? (
            <label className={`flex max-w-[280px] flex-col gap-1 ${labelClass}`}>
              {t("channels.pairing.oaDm")}
              <select
                className={selectClass}
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
            <div className="flex flex-col gap-2 border-t pt-3">
              <strong>{t("channels.pairing.confirmDenyTitle")}</strong>
              <p className="m-0 text-sm text-muted-foreground">
                {t("channels.pairing.confirmDenyPreview", { name: pairingLabel(denyRow) })}
              </p>
              <p className="m-0 text-sm text-muted-foreground">{t("channels.pairing.confirmDenyHint")}</p>
              <input
                className={fieldClass}
                value={typed}
                onChange={(e) => setTyped(e.target.value)}
                placeholder={t("channels.pairing.confirmPlaceholder")}
                aria-label={t("channels.pairing.confirmPlaceholder")}
                autoComplete="off"
                spellCheck={false}
              />
              <div className="flex flex-wrap gap-2">
                <Button variant="destructive" disabled={!denyMatched || Boolean(busy)} onClick={() => void submitDeny()}>
                  {t("channels.pairing.confirmDeny")}
                </Button>
                <Button
                  variant="outline"
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
          {pairingState.showEmpty ? <EmptyState icon="radio" title={t("channels.pairing.empty")} /> : null}
          {pairingState.showItems ? (
            <DataTable minWidth="640px" className="mt-0">
              <THead>
                <Th>{t("channels.col.name")}</Th>
                <Th>{t("channels.pairing.sender", { id: "—" })}</Th>
                <Th>{t("channels.pairing.expires", { at: "—" })}</Th>
                <Th className="text-right" />
              </THead>
              <tbody>
                {pending.map((p) => {
                  const rowBusy = busy === "pair:" + p.id;
                  return (
                    <Tr key={p.id}>
                      <Td className="font-semibold">{p.channel}</Td>
                      <Td className="text-muted-foreground">{t("channels.pairing.sender", { id: p.sender_id || "—" })}</Td>
                      <Td className="text-muted-foreground">{p.expires_at ? t("channels.pairing.expires", { at: p.expires_at }) : null}</Td>
                      <Td className="text-right" onClick={(e) => e.stopPropagation()}>
                        <div className="flex flex-wrap justify-end gap-2">
                          <Button variant="outline" size="sm" disabled={pairingBlocked || rowBusy} onClick={() => void approvePairing(p)}>
                            {t("channels.pairing.approve")}
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            className="text-destructive"
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
                      </Td>
                    </Tr>
                  );
                })}
              </tbody>
            </DataTable>
          ) : null}
        </div>
      </Card>

      {lite && catalogState.showEmpty ? (
        <p className="m-0 px-1 py-3 text-sm leading-relaxed text-muted-foreground">{t("channels.liteOff")}</p>
      ) : null}
      {agentsErr && catalogState.showItems ? (
        <p className="m-0 text-sm text-muted-foreground">{t("channels.agentsUnavailable")}</p>
      ) : null}
      {filteredEmpty ? <EmptyState icon="search" title={t("channels.filterEmpty")} /> : null}
      {catalogState.showEmpty && !lite ? <EmptyState icon="radio" title={t("channels.empty")} /> : null}
      {catalogState.showItems ? (
        <>
        <p className="m-0 text-xs text-muted-foreground">{catalogMeta == null ? "—" : t("channels.meta", { n: catalogMeta })}</p>
        <DataTable minWidth="860px">
          <THead>
            <Th>{t("channels.col.name")}</Th>
            <Th>{t("channels.col.health")}</Th>
            <Th>{t("channels.col.policy")}</Th>
            <Th>{t("channels.col.envNames")}</Th>
          </THead>
          <tbody>
            {visible.map((c) => {
              const parked = isPhase2(c);
              const writable = c.writable ?? [];
              const rem = channelRemediation(c);
              const meta = secretMetaKind(c);
              return (
                <Tr key={c.name} className="align-top">
                  <Td>
                    <div className="font-semibold">{c.name}</div>
                    <p className="mt-2 mb-0 text-xs text-muted-foreground">
                      {t("channels.col.diagnosis")}: {c.last_error ? redactPublicText(c.last_error) : "—"}
                    </p>
                    <p className="mt-1 mb-0 text-xs text-muted-foreground">
                      {t("channels.col.next")}: {t(remediateKey(rem))}
                    </p>
                    {c.name === "zalo-personal" ? (
                      <p className="mt-2 mb-0 text-xs text-muted-foreground">{t("channels.personalNoToken")}</p>
                    ) : null}
                    {parked ? (
                      <p className="mt-2 mb-0 text-xs text-muted-foreground">{t("channels.parkedNoSecret")}</p>
                    ) : catalogBlocked ? null : (
                      <div className="mt-3 flex flex-wrap items-end gap-3">
                        <label className={`flex min-w-[140px] flex-col gap-1 ${labelClass}`}>
                          {t("channels.dmPolicy")}
                          <select
                            className={selectClass}
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
                        <label className={`flex min-w-[140px] flex-col gap-1 ${labelClass}`}>
                          {t("channels.groupPolicy")}
                          <select
                            className={selectClass}
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
                        <label className="flex items-center gap-1.5 text-sm">
                          <input
                            type="checkbox"
                            checked={c.require_mention === true}
                            onChange={(e) => void patchPolicy(c.name, { require_mention: e.target.checked })}
                          />
                          {t("channels.requireMention")}
                        </label>
                        <label className={`flex min-w-[180px] flex-1 flex-col gap-1 ${labelClass}`}>
                          {t("channels.allowFrom")}
                          <textarea
                            className={fieldClass + " min-h-12 resize-y"}
                            aria-label={t("channels.allowFrom")}
                            rows={2}
                            value={allowOf(c)}
                            onChange={(e) => setAllowDrafts((prev) => ({ ...prev, [c.name]: e.target.value }))}
                          />
                          <span className="text-xs font-normal text-muted-foreground">{t("channels.allowFromHint")}</span>
                          <div>
                            <Button variant="outline" size="sm" onClick={() => void saveAllow(c)}>
                              {t("channels.savePolicy")}
                            </Button>
                          </div>
                        </label>
                      </div>
                    )}
                    {writable.length > 0 && !catalogBlocked ? (
                      <div className="mt-3 flex max-w-[520px] flex-col gap-2">
                        {c.from_env ? <p className="m-0 text-xs text-muted-foreground">{t("channels.fromEnv")}</p> : null}
                        {writable.includes("bot_token") ? (
                          <label className={`flex flex-col gap-1 ${labelClass}`}>
                            {t("channels.botToken")}
                            <input
                              type="password"
                              autoComplete="off"
                              className={fieldClass}
                              value={draftOf(c.name).bot_token}
                              onChange={(e) => setDraft(c.name, { bot_token: e.target.value })}
                            />
                            <span className="text-xs font-normal text-muted-foreground">{t("channels.botTokenHint")}</span>
                          </label>
                        ) : null}
                        {writable.includes("access_token") ? (
                          <label className={`flex flex-col gap-1 ${labelClass}`}>
                            {t("channels.accessToken")}
                            <input
                              type="password"
                              autoComplete="off"
                              className={fieldClass}
                              value={draftOf(c.name).access_token}
                              onChange={(e) => setDraft(c.name, { access_token: e.target.value })}
                            />
                          </label>
                        ) : null}
                        {writable.includes("app_secret") ? (
                          <label className={`flex flex-col gap-1 ${labelClass}`}>
                            {t("channels.appSecret")}
                            <input
                              type="password"
                              autoComplete="off"
                              className={fieldClass}
                              value={draftOf(c.name).app_secret}
                              onChange={(e) => setDraft(c.name, { app_secret: e.target.value })}
                            />
                            <span className="text-xs font-normal text-muted-foreground">{t("channels.secretHint")}</span>
                          </label>
                        ) : null}
                        <div className="flex flex-wrap gap-2">
                          <Button variant="outline" size="sm" disabled={saving === c.name} onClick={() => void saveSecrets(c)}>
                            {c.secret_set ? t("channels.rotate") : t("channels.saveSecrets")}
                          </Button>
                          {canClearBox(c) ? (
                            <Button
                              variant="ghost"
                              size="sm"
                              className="text-destructive"
                              disabled={clearing === c.name}
                              onClick={() => void clearSecrets(c)}
                            >
                              {t("channels.clear")}
                            </Button>
                          ) : null}
                          <Button variant="outline" size="sm" disabled={testing === c.name} onClick={() => void testChannel(c.name)}>
                            {t("channels.test")}
                          </Button>
                        </div>
                      </div>
                    ) : c.name === "zalo-personal" && !catalogBlocked ? (
                      <div className="mt-3">
                        <Button variant="outline" size="sm" disabled={testing === c.name} onClick={() => void testChannel(c.name)}>
                          {t("channels.test")}
                        </Button>
                      </div>
                    ) : null}
                  </Td>
                  <Td>
                    <div className="flex flex-wrap gap-1.5">
                      <StatusBadge status={c.enabled ? "success" : "default"} label={c.enabled ? t("channels.enabled") : t("channels.disabled")} />
                      {c.health ? <StatusBadge status={healthStatus(c.health)} label={c.health} /> : null}
                      {c.missing ? <StatusBadge status="warning" label={t("channels.missing")} /> : null}
                      <StatusBadge status={meta === "unset" ? "default" : "success"} label={t(secretMetaKey(meta))} />
                    </div>
                  </Td>
                  <Td onClick={(e) => e.stopPropagation()}>
                    <div className="text-xs text-muted-foreground">{[c.dm_policy, c.group_policy].filter(Boolean).join(" / ") || "—"}</div>
                    <select
                      className={selectClass + " mt-1"}
                      aria-label={t("channels.agent")}
                      value={c.bound_agent_id ?? ""}
                      disabled={catalogBlocked || Boolean(agentsErr)}
                      onChange={(e) => void bind(c.name, e.target.value)}
                    >
                      <option value="">{t("channels.agent")}</option>
                      {agents.map((a) => (
                        <option key={a.id} value={a.id}>
                          {a.display_name || a.agent_key || a.id}
                        </option>
                      ))}
                    </select>
                  </Td>
                  <Td onClick={(e) => e.stopPropagation()}>
                    <div className="flex min-w-0 flex-col gap-1.5">
                      {c.env_names.map((env) => (
                        <span key={env} className="flex min-w-0 items-center gap-2">
                          <code className="min-w-0 overflow-hidden text-ellipsis whitespace-nowrap font-mono text-xs">{env}</code>
                          <Button variant="ghost" size="sm" disabled={catalogBlocked} onClick={() => void copyEnv(env)}>
                            {copied === env ? t("channels.copied") : t("common.copy")}
                          </Button>
                        </span>
                      ))}
                    </div>
                  </Td>
                </Tr>
              );
            })}
          </tbody>
        </DataTable>
        </>
      ) : null}

      <Card>
        <CardHeader icon="hook" title={t("channels.qr")} />
        <p className="mx-4 mb-2 mt-0 text-sm text-muted-foreground">{t("channels.qr.risk")}</p>
        {qrErr ? (
          <p className="mx-4 mb-2 mt-0 text-sm text-muted-foreground">{t("channels.qr.unavailable")}</p>
        ) : (
          <p className="mx-4 mb-2 mt-0 text-sm">{t("channels.qr.status", { status: qrStatus || "—" })}</p>
        )}
        <div className="px-4 pb-4">
          <Button variant="ghost" className="text-destructive" disabled={catalogBlocked || busy === "logout"} onClick={() => void logoutPersonal()}>
            {t("channels.qr.logout")}
          </Button>
        </div>
      </Card>
    </PageChrome>
  );
}
