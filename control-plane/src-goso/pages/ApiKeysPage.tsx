import { useEffect, useMemo, useState } from "react";
import { apiKeysApi, type ApiKey } from "../api/apikeys";
import {
  SCOPES,
  asCreated,
  asPublic,
  filterKeys,
  formatWhen,
  hideCopiedSecret,
  keyConfirmMatch,
  keyLabel,
  maskedPrefix,
  publicHasSecrets,
  toggleScope,
  usageLabel,
  type LastSecret,
} from "../api/apikeys-ops";
import { classifyPageState, formatStaleAt, inventoryBlocksMutation, listMetaCount } from "../api/page-state";
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
import { fieldClass, labelClass } from "../ui/field";

function statusOf(st: string): Status {
  if (st === "active") return "success";
  if (st === "expired") return "warning";
  if (st === "revoked") return "error";
  return "default";
}

function statusKey(st: string): MsgKey {
  if (st === "revoked") return "apikeys.status.revoked";
  if (st === "expired") return "apikeys.status.expired";
  return "apikeys.status.active";
}

function scopeKey(scope: string): MsgKey {
  if (scope === "admin") return "apikeys.scope.admin";
  if (scope === "write") return "apikeys.scope.write";
  if (scope === "approvals") return "apikeys.scope.approvals";
  if (scope === "pairing") return "apikeys.scope.pairing";
  if (scope === "provision") return "apikeys.scope.provision";
  return "apikeys.scope.read";
}

function toRFC3339(local: string): string | undefined {
  const s = local.trim();
  if (!s) return undefined;
  const d = new Date(s);
  if (Number.isNaN(d.getTime())) return undefined;
  return d.toISOString();
}

export function ApiKeysPage() {
  const { t, locale } = useI18n();
  const [rows, setRows] = useState<ApiKey[]>([]);
  const [selected, setSelected] = useState("");
  const [q, setQ] = useState("");
  const [loading, setLoading] = useState(true);
  const [loaded, setLoaded] = useState(false);
  const [loadedAt, setLoadedAt] = useState<string | null>(null);
  const [err, setErr] = useState<unknown>(null);
  const [actionErr, setActionErr] = useState("");
  const [ok, setOk] = useState("");
  const [busy, setBusy] = useState("");
  const [name, setName] = useState("");
  const [tenant, setTenant] = useState("");
  const [scopes, setScopes] = useState<string[]>(["read"]);
  const [expires, setExpires] = useState("");
  const [last, setLast] = useState<LastSecret | null>(null);
  const [copied, setCopied] = useState("");
  const [confirm, setConfirm] = useState<ApiKey | null>(null);
  const [typed, setTyped] = useState("");
  const na = t("apikeys.na");

  const state = classifyPageState({
    loading,
    loaded,
    error: err,
    itemCount: rows.length,
    keepStale: loaded && rows.length > 0,
  });
  const blocked = inventoryBlocksMutation(state.kind);
  const filtersOn = q.trim().length > 0;
  const filtered = useMemo(() => filterKeys(state.showItems ? rows : [], q), [rows, q, state.showItems]);
  const trueEmpty = state.showEmpty && !filtersOn;
  const filterEmpty = state.showEmpty && filtersOn;
  const metaN = listMetaCount(state.kind, rows.length);
  const detail = filtered.find((r) => r.id === selected) || filtered[0] || null;
  const matched = confirm ? keyConfirmMatch(typed, confirm) : false;

  async function load(keepId = selected) {
    setLoading(true);
    try {
      const j = await apiKeysApi.list(q);
      const leak = (j.keys || []).some((row) => publicHasSecrets(row));
      setRows(asPublic(j.keys));
      setLoaded(true);
      setLoadedAt(new Date().toISOString());
      setErr(null);
      setActionErr(leak ? t("apikeys.leak") : "");
      const id = keepId && j.keys.some((r) => r.id === keepId) ? keepId : j.keys[0]?.id || "";
      setSelected(id);
    } catch (e) {
      setErr(e);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load("");
  }, [q]);

  useEffect(() => {
    return () => {
      setLast(null);
    };
  }, []);

  async function createKey() {
    if (blocked) return;
    setBusy("create");
    setOk("");
    try {
      const created = asCreated(
        await apiKeysApi.create({
          name: name.trim(),
          tenant_id: tenant.trim() || undefined,
          scopes,
          expires_at: toRFC3339(expires),
        }),
      );
      if (!created) {
        setActionErr(t("apikeys.leak"));
        return;
      }
      setLast(created);
      setCopied("");
      setName("");
      setTenant("");
      setScopes(["read"]);
      setExpires("");
      setActionErr("");
      setOk(t("apikeys.createOk"));
      await load(created.id);
    } catch (e) {
      setActionErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  async function copySecret() {
    if (!last?.secret) return;
    try {
      await navigator.clipboard.writeText(last.secret);
      setLast(hideCopiedSecret(last));
      setCopied(t("apikeys.copied"));
    } catch (e) {
      setActionErr(formatPublicError(e));
    }
  }

  function hideSecret() {
    if (!last) return;
    setLast(hideCopiedSecret(last));
    setCopied("");
  }

  async function submitRevoke() {
    if (blocked || !confirm || !keyConfirmMatch(typed, confirm)) {
      setActionErr(t("apikeys.mismatch"));
      return;
    }
    setBusy("revoke");
    try {
      await apiKeysApi.revoke(confirm.id, typed.trim());
      setOk(t("apikeys.revokeOk"));
      setActionErr("");
      if (last?.id === confirm.id) setLast(null);
      setConfirm(null);
      setTyped("");
      await load(confirm.id);
    } catch (e) {
      setActionErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  return (
    <PageChrome
      icon="key"
      title={t("apikeys.title")}
      description={t("apikeys.desc")}
      primary={
        <Button
          variant="default"
          icon="plus"
          disabled={blocked || Boolean(busy) || !name.trim() || scopes.length === 0}
          onClick={() => void createKey()}
        >
          {t("apikeys.create")}
        </Button>
      }
      refresh={
        <Button variant="outline" icon="refresh" iconGesture onClick={() => void load(selected)} disabled={loading || Boolean(busy)}>
          {t("common.refresh")}
        </Button>
      }
      filters={<SearchInput value={q} onChange={setQ} placeholder={t("apikeys.search")} aria-label={t("apikeys.search")} />}
    >
      <p className="text-sm text-muted-foreground">{t("apikeys.once")}</p>
      <PageStatus kind={state.kind} errorText={err ? formatPublicError(err) : ""} staleAt={formatStaleAt(loadedAt, locale)} onReload={() => void load(selected)} />
      {actionErr ? <StatusLine kind="error">{actionErr}</StatusLine> : null}
      {ok && !actionErr ? (
        <p role="status" className="text-sm text-success">
          {ok}
        </p>
      ) : null}
      {!blocked ? (
        <Card>
          <CardHeader icon="plus" title={t("apikeys.create")} />
          <div className="flex flex-col gap-3 p-4">
            <div className="flex flex-wrap gap-3">
              <label className={labelClass + " min-w-[160px] flex-1"}>
                {t("apikeys.name")}
                <input
                  className={fieldClass + " mt-1"}
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder={t("apikeys.name")}
                  aria-label={t("apikeys.name")}
                  autoComplete="off"
                  spellCheck={false}
                />
              </label>
              <label className={labelClass + " min-w-[120px]"}>
                {t("apikeys.tenant")}
                <input
                  className={fieldClass + " mt-1"}
                  value={tenant}
                  onChange={(e) => setTenant(e.target.value)}
                  placeholder={t("apikeys.tenant")}
                  aria-label={t("apikeys.tenant")}
                  autoComplete="off"
                  spellCheck={false}
                />
              </label>
              <label className={labelClass + " min-w-[180px]"}>
                {t("apikeys.expires")}
                <input
                  className={fieldClass + " mt-1"}
                  type="datetime-local"
                  value={expires}
                  onChange={(e) => setExpires(e.target.value)}
                  aria-label={t("apikeys.expires")}
                />
              </label>
            </div>
            <fieldset className="m-0 flex flex-wrap gap-2.5 border-0 p-0">
              <legend className="p-0 text-sm font-medium">{t("apikeys.scopes")}</legend>
              {SCOPES.map((s) => (
                <label key={s} className="flex items-center gap-1.5 text-sm">
                  <input type="checkbox" checked={scopes.includes(s)} onChange={() => setScopes(toggleScope(scopes, s))} />
                  {t(scopeKey(s))}
                </label>
              ))}
            </fieldset>
          </div>
        </Card>
      ) : null}
      {!blocked ? (
        <Card>
          <CardHeader icon="lock" title={t("apikeys.last")} />
          {!last ? (
            <EmptyState icon="key" title={t("apikeys.lastEmpty")} />
          ) : (
            <div className="flex flex-col gap-2 p-4 text-sm">
              <p className="text-muted-foreground">{t("apikeys.secretOnce")}</p>
              {copied ? (
                <p role="status" className="text-success">
                  {copied}
                </p>
              ) : null}
              <div className="flex items-center gap-2">
                <span className="w-[110px] font-semibold text-muted-foreground">{t("apikeys.col.prefix")}</span>
                <code className="text-xs">{last.prefix}</code>
              </div>
              <div className="flex flex-wrap items-center gap-2">
                <span className="w-[110px] font-semibold text-muted-foreground">{t("apikeys.secret")}</span>
                <code className="flex-1 overflow-anywhere text-xs">{last.secret ?? t("apikeys.redacted")}</code>
                {last.secret ? (
                  <>
                    <Button variant="outline" size="sm" onClick={() => void copySecret()}>
                      {t("common.copy")}
                    </Button>
                    <Button variant="outline" size="sm" onClick={hideSecret}>
                      {t("apikeys.hide")}
                    </Button>
                  </>
                ) : null}
              </div>
            </div>
          )}
        </Card>
      ) : null}
      {confirm && !blocked ? (
        <Card>
          <CardHeader icon="lock" title={t("apikeys.confirmTitle")} />
          <div className="flex flex-col gap-3 p-4">
            <p className="text-sm">{t("apikeys.confirmPreview", { name: keyLabel(confirm) })}</p>
            <p className="text-sm text-muted-foreground">{t("apikeys.confirmHint")}</p>
            <input
              className={fieldClass}
              value={typed}
              onChange={(e) => setTyped(e.target.value)}
              placeholder={t("apikeys.confirmPlaceholder")}
              aria-label={t("apikeys.confirmPlaceholder")}
              autoComplete="off"
              spellCheck={false}
            />
            <div className="flex flex-wrap gap-2">
              <Button variant="destructive" disabled={!matched || Boolean(busy)} onClick={() => void submitRevoke()}>
                {t("apikeys.confirmRevoke")}
              </Button>
              <Button
                variant="outline"
                disabled={Boolean(busy)}
                onClick={() => {
                  setConfirm(null);
                  setTyped("");
                }}
              >
                {t("apikeys.cancel")}
              </Button>
            </div>
          </div>
        </Card>
      ) : null}
      {trueEmpty ? <EmptyState icon="key" title={t("apikeys.empty")} data-page-state="empty" /> : null}
      {filterEmpty ? <EmptyState icon="search" title={t("apikeys.filterEmpty")} data-page-state="filtered_empty" /> : null}
      {filtered.length > 0 ? (
        <>
          <div className="flex items-center justify-between">
            <h2 className="text-sm font-semibold">{t("apikeys.list")}</h2>
            <span className="text-xs text-muted-foreground">{metaN == null ? "—" : t("apikeys.meta", { n: metaN })}</span>
          </div>
          <DataTable minWidth="800px">
            <THead>
              <Th>{t("apikeys.col.name")}</Th>
              <Th>{t("apikeys.col.prefix")}</Th>
              <Th>{t("apikeys.col.scopes")}</Th>
              <Th>{t("apikeys.col.status")}</Th>
              <Th>{t("apikeys.col.usage")}</Th>
              <Th className="text-right">{t("apikeys.col.actions")}</Th>
            </THead>
            <tbody>
              {filtered.map((row) => {
                const on = row.id === (detail?.id || selected);
                return (
                  <Tr
                    key={row.id}
                    selected={on}
                    onClick={() => {
                      if (blocked) return;
                      setSelected(row.id);
                      setConfirm(null);
                      setTyped("");
                    }}
                  >
                    <Td className="font-semibold">{row.name || row.id}</Td>
                    <Td>
                      <code className="text-xs">{maskedPrefix(row.prefix)}</code>
                    </Td>
                    <Td className="text-muted-foreground">{row.scopes.join(", ") || na}</Td>
                    <Td>
                      <StatusBadge status={statusOf(row.status)} label={t(statusKey(row.status))} />
                    </Td>
                    <Td className="text-muted-foreground">{usageLabel(row, t("apikeys.never"))}</Td>
                    <Td onClick={(e) => e.stopPropagation()}>
                      <div className="flex justify-end">
                        <Button
                          variant="ghost"
                          size="sm"
                          className="text-destructive"
                          disabled={blocked || row.status === "revoked" || busy === "revoke"}
                          onClick={() => {
                            setConfirm(row);
                            setTyped("");
                            setSelected(row.id);
                          }}
                        >
                          {t("apikeys.revoke")}
                        </Button>
                      </div>
                    </Td>
                  </Tr>
                );
              })}
            </tbody>
          </DataTable>
        </>
      ) : null}
      {detail && state.showItems ? (
        <Card>
          <CardHeader icon="pulse" title={t("apikeys.usageTitle")} meta={keyLabel(detail)} />
          <div className="grid gap-1.5 p-4 text-sm">
            <div>
              {t("apikeys.col.prefix")}: <code>{maskedPrefix(detail.prefix)}</code>
            </div>
            <div>
              {t("apikeys.tenant")}: {detail.tenant_id || na}
            </div>
            <div>
              {t("apikeys.col.scopes")}: {detail.scopes.map((s) => t(scopeKey(s))).join(", ") || na}
            </div>
            <div>
              {t("apikeys.col.expiry")}: {formatWhen(detail.expires_at, t("apikeys.neverExpires"))}
            </div>
            <div>
              {t("apikeys.created")}: {formatWhen(detail.created_at, na)}
            </div>
            <div>
              {t("apikeys.col.usage")}: {usageLabel(detail, t("apikeys.never"))}
            </div>
          </div>
        </Card>
      ) : null}
    </PageChrome>
  );
}
