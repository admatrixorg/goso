import { useEffect, useMemo, useState, type CSSProperties } from "react";
import { disposeOneTimeSecrets, filterByQuery } from "../api/capabilities-ops";
import { classifyPageState, formatStaleAt, inventoryBlocksMutation, isFilteredEmpty, listMetaCount } from "../api/page-state";
import {
  asCreated,
  asPublic,
  canReplay,
  canTestOrReplay,
  hideCopiedSecret,
  lastDeliveryLabel,
  listTargetName,
  webhookEndpoint,
  webhookStatus,
  type LastSecret,
} from "../api/webhooks-ops";
import { webhooksApi, type WebhookPublic } from "../api/webhooks";
import { useI18n, type MsgKey } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { PageChrome } from "../ui/PageChrome";
import { PageStatus } from "../ui/PageStatus";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

const fieldStyle: CSSProperties = {
  fontSize: 12.5,
  padding: "6px 10px",
  borderRadius: 8,
  border: "1px solid var(--border)",
  background: "var(--card)",
  color: "var(--text)",
  width: "100%",
  boxSizing: "border-box",
};

function statusTone(status: string): "positive" | "warning" | "critical" | "neutral" {
  if (status === "active") return "positive";
  if (status === "failing") return "warning";
  if (status === "revoked") return "critical";
  return "neutral";
}

function statusKey(status: string): MsgKey {
  if (status === "revoked") return "webhooks.status.revoked";
  if (status === "failing") return "webhooks.status.failing";
  return "webhooks.status.active";
}

export function WebhooksPage() {
  const { t, locale } = useI18n();
  const [rows, setRows] = useState<WebhookPublic[]>([]);
  const [last, setLast] = useState<LastSecret | null>(null);
  const [err, setErr] = useState("");
  const [ok, setOk] = useState("");
  const [loading, setLoading] = useState(true);
  const [loaded, setLoaded] = useState(false);
  const [loadedAt, setLoadedAt] = useState<string | null>(null);
  const [loadErr, setLoadErr] = useState<unknown>(null);
  const [copied, setCopied] = useState("");
  const [busy, setBusy] = useState("");
  const [name, setName] = useState("");
  const [endpoint, setEndpoint] = useState("");
  const [query, setQuery] = useState("");
  const [showForm, setShowForm] = useState(false);
  const state = classifyPageState({
    loading,
    loaded,
    error: loadErr,
    itemCount: rows.length,
    keepStale: loaded && rows.length > 0,
  });
  const blocked = inventoryBlocksMutation(state.kind);
  const visible = useMemo(
    () => filterByQuery(state.showItems ? rows : [], query, (w) => `${w.name} ${w.id} ${webhookEndpoint(w)} ${webhookStatus(w)}`),
    [rows, query, state.showItems],
  );
  const filteredEmpty = isFilteredEmpty(state, rows.length, visible.length);
  const metaN = listMetaCount(state.kind, visible.length);

  async function load() {
    setLoading(true);
    try {
      const j = await webhooksApi.list();
      setRows(asPublic(j.webhooks));
      setLoaded(true);
      setLoadedAt(new Date().toISOString());
      setLoadErr(null);
      setErr("");
    } catch (e) {
      setLoadErr(e);
    } finally {
      setLoading(false);
    }
  }
  useEffect(() => {
    void load();
    return () => {
      setLast((cur) => disposeOneTimeSecrets(cur));
    };
  }, []);

  async function create() {
    if (blocked) return;
    setBusy("create");
    try {
      const created = asCreated(
        await webhooksApi.create({
          name: name.trim() || undefined,
          endpoint: endpoint.trim() || undefined,
        }),
      );
      setLast(created);
      setCopied("");
      setOk("");
      setErr("");
      setName("");
      setEndpoint("");
      setShowForm(false);
      setOk(t("webhooks.createdOk"));
      await load();
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  async function rotate(row: WebhookPublic) {
    if (blocked) return;
    const named = listTargetName(row);
    if (!window.confirm(t("webhooks.confirmRotate", { name: named }))) return;
    setBusy("rotate:" + row.id);
    try {
      const created = asCreated(await webhooksApi.rotate(row.id));
      created.note = t("webhooks.rotated");
      setLast(created);
      setCopied("");
      setOk(t("webhooks.rotated"));
      setErr("");
      await load();
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  async function revoke(row: WebhookPublic) {
    if (blocked) return;
    const named = listTargetName(row);
    if (!window.confirm(t("webhooks.confirmRevoke", { name: named }))) return;
    setBusy("revoke:" + row.id);
    try {
      await webhooksApi.revoke(row.id);
      setOk(t("webhooks.revokedOk"));
      setErr("");
      if (last?.id === row.id) {
        setLast(null);
      }
      await load();
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  async function test(row: WebhookPublic) {
    if (blocked) return;
    if (!canTestOrReplay(row)) {
      setErr(t("webhooks.testBlocked"));
      return;
    }
    setBusy("test:" + row.id);
    try {
      await webhooksApi.test(row.id);
      setOk(t("webhooks.testedOk"));
      setErr("");
      await load();
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  async function replay(row: WebhookPublic) {
    if (blocked) return;
    if (!canReplay(row)) {
      setErr(t("webhooks.testBlocked"));
      return;
    }
    setBusy("replay:" + row.id);
    try {
      await webhooksApi.replay(row.id);
      setOk(t("webhooks.replayedOk"));
      setErr("");
      await load();
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  async function copy(kind: "token" | "hmac") {
    if (!last) return;
    const value = kind === "token" ? last.token : last.hmac_key;
    if (!value) return;
    try {
      await navigator.clipboard.writeText(value);
      setLast(hideCopiedSecret(last, kind));
      setCopied(t("webhooks.copied"));
    } catch (e) {
      setErr(formatPublicError(e));
    }
  }

  return (
    <PageChrome
      icon="hook"
      title={t("webhooks.title")}
      description={t("webhooks.desc")}
      primary={
        <Button
          variant="primary"
          icon="plus"
          disabled={blocked}
          onClick={() => {
            if (blocked) return;
            setShowForm(true);
            setErr("");
            setOk("");
          }}
        >
          {t("webhooks.create")}
        </Button>
      }
      refresh={
        <Button icon="refresh" iconGesture onClick={() => void load()} disabled={loading}>
          {t("common.refresh")}
        </Button>
      }
      filters={
        state.showItems || state.showEmpty ? (
          <input
            className="z-field"
            style={{ flex: 1, minWidth: 160 }}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={t("webhooks.search")}
            aria-label={t("webhooks.search")}
            autoComplete="off"
          />
        ) : null
      }
    >
      <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("webhooks.notHooks")}</p>
      <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("webhooks.noSecrets")}</p>
      <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("webhooks.testBlocked")}</p>
      <PageStatus kind={state.kind} errorText={loadErr ? formatPublicError(loadErr) : ""} staleAt={formatStaleAt(loadedAt, locale)} onReload={() => void load()} />
      {err ? <StatusLine kind="error">{err}</StatusLine> : null}
      {ok && !err ? (
        <p role="status" style={{ margin: 0, fontSize: 12.5, color: "var(--green)" }}>
          {ok}
        </p>
      ) : null}
      {!blocked && showForm ? (
        <Card>
          <CardHeader icon="plus" title={t("webhooks.create")} />
          <div style={{ padding: "12px 16px", display: "flex", flexDirection: "column", gap: 10 }}>
            <label style={{ display: "flex", flexDirection: "column", gap: 4, fontSize: 12 }}>
              <span style={{ color: "var(--text-3)", fontWeight: 600 }}>{t("webhooks.name")}</span>
              <input value={name} onChange={(e) => setName(e.target.value)} style={fieldStyle} />
            </label>
            <label style={{ display: "flex", flexDirection: "column", gap: 4, fontSize: 12 }}>
              <span style={{ color: "var(--text-3)", fontWeight: 600 }}>{t("webhooks.endpoint")}</span>
              <input
                value={endpoint}
                onChange={(e) => setEndpoint(e.target.value)}
                placeholder={t("webhooks.endpointPlaceholder")}
                style={fieldStyle}
                autoComplete="off"
              />
              <span style={{ color: "var(--text-4)", fontSize: 11.5 }}>{t("webhooks.endpointHint")}</span>
            </label>
            <div style={{ display: "flex", gap: 8 }}>
              <Button variant="primary" icon="plus" disabled={busy === "create"} onClick={() => void create()}>
                {t("webhooks.create")}
              </Button>
              <Button variant="quiet" onClick={() => setShowForm(false)}>
                {t("common.cancel")}
              </Button>
            </div>
          </div>
        </Card>
      ) : null}
      <Card>
        <CardHeader icon="hook" title={t("webhooks.list")} meta={metaN == null ? "—" : t("webhooks.meta", { n: metaN })} />
        <TableScroll>
        <div style={{ display: "flex", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)", gap: 8 }}>
          <span style={{ width: 90 }}>{t("webhooks.col.status")}</span>
          <span style={{ flex: 2 }}>{t("webhooks.col.endpoint")}</span>
          <span style={{ flex: 2 }}>{t("webhooks.col.last")}</span>
          <span style={{ width: 220 }}>{t("webhooks.col.actions")}</span>
        </div>
        {state.showItems
          ? visible.map((w) => {
              const status = webhookStatus(w);
              const lastLabel = lastDeliveryLabel(w);
              const outbound = canTestOrReplay(w);
              const replayable = canReplay(w);
              return (
                <div key={w.id} style={{ display: "flex", padding: "11px 16px", fontSize: 12.5, borderBottom: "1px solid var(--border-soft)", gap: 8, alignItems: "center" }}>
                  <span style={{ width: 90 }}>
                    <Badge tone={statusTone(status)}>{t(statusKey(status))}</Badge>
                  </span>
                  <code style={{ flex: 2, fontSize: 12, overflow: "hidden", textOverflow: "ellipsis" }}>{webhookEndpoint(w)}</code>
                  <span style={{ flex: 2, color: "var(--text-3)" }}>{lastLabel || t("webhooks.never")}</span>
                  <span style={{ width: 220, display: "flex", gap: 6, flexWrap: "wrap" }}>
                    <Button disabled={blocked || busy === "test:" + w.id || !outbound} onClick={() => void test(w)} style={{ padding: "4px 10px" }}>
                      {t("webhooks.test")}
                    </Button>
                    <Button disabled={blocked || busy === "replay:" + w.id || !replayable} onClick={() => void replay(w)} style={{ padding: "4px 10px" }}>
                      {t("webhooks.replay")}
                    </Button>
                    <Button disabled={blocked || busy === "rotate:" + w.id || status === "revoked"} onClick={() => void rotate(w)} style={{ padding: "4px 10px" }}>
                      {t("webhooks.rotate")}
                    </Button>
                    <Button disabled={blocked || busy === "revoke:" + w.id || status === "revoked"} onClick={() => void revoke(w)} style={{ padding: "4px 10px" }}>
                      {t("webhooks.revoke")}
                    </Button>
                  </span>
                </div>
              );
            })
          : null}
        {state.showEmpty ? <EmptyState data-page-state="empty">{t("webhooks.empty")}</EmptyState> : null}
        {filteredEmpty ? <EmptyState data-page-state="filtered_empty">{t("webhooks.filterEmpty")}</EmptyState> : null}
        </TableScroll>
      </Card>
      {last ? (
        <Card>
          <CardHeader icon="lock" title={t("webhooks.last")} />
          <div style={{ padding: "12px 16px", display: "flex", flexDirection: "column", gap: 10, fontSize: 12.5 }}>
            <p style={{ margin: 0, color: "var(--text-3)" }}>{t("webhooks.secretOnce")}</p>
            {copied ? <p style={{ margin: 0, color: "var(--green)" }}>{copied}</p> : null}
            {last.note ? <p style={{ margin: 0, color: "var(--green)" }}>{last.note}</p> : null}
            <Row label={t("webhooks.id")} value={last.id} />
            <Row label={t("webhooks.prefix")} value={last.token_prefix} />
            <div style={{ display: "flex", gap: 8, alignItems: "center", flexWrap: "wrap" }}>
              <span style={{ width: 110, color: "var(--text-3)", fontWeight: 600 }}>{t("webhooks.token")}</span>
              <code style={{ flex: 1, fontSize: 12 }}>{last.token ?? t("webhooks.redacted")}</code>
              {last.token ? (
                <Button onClick={() => void copy("token")} style={{ padding: "4px 10px" }}>
                  {t("common.copy")}
                </Button>
              ) : null}
            </div>
            <div style={{ display: "flex", gap: 8, alignItems: "center", flexWrap: "wrap" }}>
              <span style={{ width: 110, color: "var(--text-3)", fontWeight: 600 }}>{t("webhooks.hmac")}</span>
              <code style={{ flex: 1, fontSize: 12 }}>{last.hmac_key ?? t("webhooks.redacted")}</code>
              {last.hmac_key ? (
                <Button onClick={() => void copy("hmac")} style={{ padding: "4px 10px" }}>
                  {t("common.copy")}
                </Button>
              ) : null}
            </div>
            {last.token || last.hmac_key ? (
              <Button
                variant="quiet"
                onClick={() => {
                  setLast(disposeOneTimeSecrets(last));
                  setCopied(t("webhooks.hide"));
                }}
              >
                {t("webhooks.hide")}
              </Button>
            ) : null}
          </div>
        </Card>
      ) : null}
    </PageChrome>
  );
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
      <span style={{ width: 110, color: "var(--text-3)", fontWeight: 600 }}>{label}</span>
      <code style={{ fontSize: 12 }}>{value}</code>
    </div>
  );
}
