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
import { useI18n, type MsgKey } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { SectionHeader } from "../ui/SectionHeader";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

function statusTone(st: string): "positive" | "warning" | "critical" | "neutral" {
  if (st === "active") return "positive";
  if (st === "expired") return "warning";
  if (st === "revoked") return "critical";
  return "neutral";
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
  const { t } = useI18n();
  const [rows, setRows] = useState<ApiKey[]>([]);
  const [selected, setSelected] = useState("");
  const [q, setQ] = useState("");
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState("");
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

  async function load(keepId = selected) {
    setLoading(true);
    try {
      const j = await apiKeysApi.list(q);
      const leak = (j.keys || []).some((row) => publicHasSecrets(row));
      setRows(asPublic(j.keys));
      setErr(leak ? t("apikeys.leak") : "");
      const id = keepId && j.keys.some((r) => r.id === keepId) ? keepId : j.keys[0]?.id || "";
      setSelected(id);
    } catch (e) {
      setErr(formatPublicError(e));
      setRows([]);
      setSelected("");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load("");
  }, [q]);

  const filtered = useMemo(() => filterKeys(rows, q), [rows, q]);
  const detail = filtered.find((r) => r.id === selected) || filtered[0] || null;
  const matched = confirm ? keyConfirmMatch(typed, confirm) : false;
  const empty = !loading && !err && filtered.length === 0;

  async function createKey() {
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
        setErr(t("apikeys.leak"));
        return;
      }
      setLast(created);
      setCopied("");
      setName("");
      setTenant("");
      setScopes(["read"]);
      setExpires("");
      setErr("");
      setOk(t("apikeys.createOk"));
      await load(created.id);
    } catch (e) {
      setErr(formatPublicError(e));
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
      setErr(formatPublicError(e));
    }
  }

  async function submitRevoke() {
    if (!confirm || !keyConfirmMatch(typed, confirm)) {
      setErr(t("apikeys.mismatch"));
      return;
    }
    setBusy("revoke");
    try {
      await apiKeysApi.revoke(confirm.id, typed.trim());
      setOk(t("apikeys.revokeOk"));
      setErr("");
      if (last?.id === confirm.id) setLast(null);
      setConfirm(null);
      setTyped("");
      await load(confirm.id);
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  return (
    <div style={{ padding: "14px 22px 40px", display: "flex", flexDirection: "column", gap: 14 }}>
      <SectionHeader
        icon="lock"
        title={t("apikeys.title")}
        description={t("apikeys.desc")}
        actions={
          <Button icon="refresh" iconGesture onClick={() => void load(selected)} disabled={loading || Boolean(busy)}>
            {t("common.refresh")}
          </Button>
        }
      />
      <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("apikeys.once")}</p>
      {loading ? <StatusLine kind="loading" /> : null}
      {err ? <StatusLine kind="error">{err}</StatusLine> : null}
      {ok && !err ? (
        <p role="status" style={{ margin: 0, fontSize: 12.5, color: "var(--green)" }}>
          {ok}
        </p>
      ) : null}
      <Card>
        <CardHeader icon="plus" title={t("apikeys.create")} />
        <div style={{ padding: "0 16px 16px", display: "flex", flexDirection: "column", gap: 10 }}>
          <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
            <input
              className="z-field"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={t("apikeys.name")}
              aria-label={t("apikeys.name")}
              autoComplete="off"
              spellCheck={false}
              style={{ minWidth: 160, flex: 1 }}
            />
            <input
              className="z-field"
              value={tenant}
              onChange={(e) => setTenant(e.target.value)}
              placeholder={t("apikeys.tenant")}
              aria-label={t("apikeys.tenant")}
              autoComplete="off"
              spellCheck={false}
              style={{ minWidth: 120 }}
            />
            <input
              className="z-field"
              type="datetime-local"
              value={expires}
              onChange={(e) => setExpires(e.target.value)}
              aria-label={t("apikeys.expires")}
              style={{ minWidth: 180 }}
            />
          </div>
          <fieldset style={{ border: "none", margin: 0, padding: 0, display: "flex", gap: 10, flexWrap: "wrap" }}>
            <legend style={{ fontSize: 12, color: "var(--text-3)", fontWeight: 600, padding: 0 }}>{t("apikeys.scopes")}</legend>
            {SCOPES.map((s) => (
              <label key={s} style={{ display: "flex", alignItems: "center", gap: 6, fontSize: 12.5, color: "var(--text-2)" }}>
                <input
                  type="checkbox"
                  checked={scopes.includes(s)}
                  onChange={() => setScopes(toggleScope(scopes, s))}
                />
                {t(scopeKey(s))}
              </label>
            ))}
          </fieldset>
          <div>
            <Button variant="accent" disabled={Boolean(busy) || !name.trim() || scopes.length === 0} onClick={() => void createKey()}>
              {t("common.create")}
            </Button>
          </div>
        </div>
      </Card>
      <Card>
        <CardHeader icon="lock" title={t("apikeys.last")} />
        {!last ? (
          <EmptyState>{t("apikeys.lastEmpty")}</EmptyState>
        ) : (
          <div style={{ padding: "0 16px 16px", display: "flex", flexDirection: "column", gap: 8, fontSize: 12.5 }}>
            <p style={{ margin: 0, color: "var(--text-3)" }}>{t("apikeys.secretOnce")}</p>
            {copied ? (
              <p role="status" style={{ margin: 0, color: "var(--green)" }}>
                {copied}
              </p>
            ) : null}
            <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
              <span style={{ width: 110, color: "var(--text-3)", fontWeight: 600 }}>{t("apikeys.col.prefix")}</span>
              <code style={{ fontSize: 12 }}>{last.prefix}</code>
            </div>
            <div style={{ display: "flex", gap: 8, alignItems: "center", flexWrap: "wrap" }}>
              <span style={{ width: 110, color: "var(--text-3)", fontWeight: 600 }}>{t("apikeys.secret")}</span>
              <code style={{ flex: 1, fontSize: 12, overflowWrap: "anywhere" }}>{last.secret ?? t("apikeys.redacted")}</code>
              {last.secret ? (
                <Button onClick={() => void copySecret()} style={{ padding: "4px 10px" }}>
                  {t("common.copy")}
                </Button>
              ) : null}
            </div>
          </div>
        )}
      </Card>
      {confirm ? (
        <Card>
          <CardHeader icon="lock" title={t("apikeys.confirmTitle")} />
          <div style={{ padding: "0 16px 16px", display: "flex", flexDirection: "column", gap: 10 }}>
            <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-2)" }}>
              {t("apikeys.confirmPreview", { name: keyLabel(confirm) })}
            </p>
            <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("apikeys.confirmHint")}</p>
            <input
              className="z-field"
              value={typed}
              onChange={(e) => setTyped(e.target.value)}
              placeholder={t("apikeys.confirmPlaceholder")}
              aria-label={t("apikeys.confirmPlaceholder")}
              autoComplete="off"
              spellCheck={false}
            />
            <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
              <Button variant="accent" disabled={!matched || Boolean(busy)} onClick={() => void submitRevoke()}>
                {t("apikeys.confirmRevoke")}
              </Button>
              <Button
                variant="quiet"
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
      <Card>
        <CardHeader icon="list" title={t("apikeys.list")} meta={t("apikeys.meta", { n: filtered.length })} />
        <div style={{ padding: "0 16px 10px" }}>
          <input
            className="z-field"
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder={t("apikeys.search")}
            aria-label={t("apikeys.search")}
            autoComplete="off"
            style={{ width: "100%" }}
          />
        </div>
        <TableScroll>
          <div
            style={{
              display: "flex",
              padding: "8px 16px",
              borderBottom: "1px solid var(--border-soft)",
              fontSize: 10,
              fontWeight: 600,
              letterSpacing: ".4px",
              color: "var(--text-3)",
              gap: 8,
            }}
          >
            <span style={{ flex: 1.2 }}>{t("apikeys.col.name")}</span>
            <span style={{ flex: 1 }}>{t("apikeys.col.prefix")}</span>
            <span style={{ flex: 1.4 }}>{t("apikeys.col.scopes")}</span>
            <span style={{ flex: 0.8 }}>{t("apikeys.col.status")}</span>
            <span style={{ flex: 1.2 }}>{t("apikeys.col.usage")}</span>
            <span style={{ width: 88 }}>{t("apikeys.col.actions")}</span>
          </div>
          {empty ? <EmptyState>{q.trim() ? t("apikeys.filterEmpty") : t("apikeys.empty")}</EmptyState> : null}
          {filtered.map((row) => {
            const on = row.id === (detail?.id || selected);
            return (
              <div
                key={row.id}
                style={{
                  display: "flex",
                  alignItems: "center",
                  gap: 8,
                  width: "100%",
                  borderBottom: "1px solid var(--border-soft)",
                  background: on ? "var(--accent-soft)" : "transparent",
                }}
              >
                <button
                  type="button"
                  onClick={() => {
                    setSelected(row.id);
                    setConfirm(null);
                    setTyped("");
                  }}
                  style={{
                    display: "flex",
                    alignItems: "center",
                    gap: 8,
                    flex: 1,
                    padding: "11px 16px",
                    fontSize: 12.5,
                    border: "none",
                    background: "transparent",
                    color: "var(--text)",
                    textAlign: "left",
                    cursor: "pointer",
                  }}
                >
                  <span style={{ flex: 1.2, fontWeight: 600 }}>{row.name || row.id}</span>
                  <code style={{ flex: 1, fontSize: 12 }}>{maskedPrefix(row.prefix)}</code>
                  <span style={{ flex: 1.4, color: "var(--text-3)" }}>{row.scopes.join(", ") || na}</span>
                  <span style={{ flex: 0.8 }}>
                    <Badge tone={statusTone(row.status)}>{t(statusKey(row.status))}</Badge>
                  </span>
                  <span style={{ flex: 1.2, color: "var(--text-3)" }}>{usageLabel(row, t("apikeys.never"))}</span>
                </button>
                <span style={{ width: 88, flex: "none", paddingRight: 16 }}>
                  <Button
                    disabled={row.status === "revoked" || busy === "revoke"}
                    onClick={() => {
                      setConfirm(row);
                      setTyped("");
                      setSelected(row.id);
                    }}
                    style={{ padding: "4px 10px" }}
                  >
                    {t("apikeys.revoke")}
                  </Button>
                </span>
              </div>
            );
          })}
        </TableScroll>
      </Card>
      {detail ? (
        <Card>
          <CardHeader icon="pulse" title={t("apikeys.usageTitle")} meta={keyLabel(detail)} />
          <div style={{ padding: "0 16px 16px", display: "grid", gap: 6, fontSize: 12.5, color: "var(--text-2)" }}>
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
    </div>
  );
}
