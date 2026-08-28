import { useEffect, useState } from "react";
import { webhooksApi, type WebhookCreated, type WebhookPublic } from "../api/webhooks";
import { useI18n } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { SectionHeader } from "../ui/SectionHeader";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

type LastSecret = {
  id: string;
  token_prefix: string;
  token?: string;
  hmac_key?: string;
  note?: string;
};

function asCreated(j: WebhookCreated): LastSecret {
  return {
    id: typeof j.id === "string" ? j.id : "",
    token_prefix: typeof j.token_prefix === "string" ? j.token_prefix : "",
    token: typeof j.token === "string" ? j.token : undefined,
    hmac_key: typeof j.hmac_key === "string" ? j.hmac_key : undefined,
  };
}

function asPublic(rows: WebhookPublic[] | undefined): WebhookPublic[] {
  return (rows ?? [])
    .map((w) => ({
      id: typeof w?.id === "string" ? w.id : "",
      token_prefix: typeof w?.token_prefix === "string" ? w.token_prefix : "",
      name: typeof w?.name === "string" ? w.name : "",
      kind: typeof w?.kind === "string" ? w.kind : "llm",
      agent_id: typeof w?.agent_id === "string" ? w.agent_id : "",
      require_hmac: Boolean(w?.require_hmac),
      revoked: Boolean(w?.revoked),
    }))
    .filter((w) => w.id);
}

export function WebhooksPage() {
  const { t } = useI18n();
  const [rows, setRows] = useState<WebhookPublic[]>([]);
  const [last, setLast] = useState<LastSecret | null>(null);
  const [err, setErr] = useState("");
  const [ok, setOk] = useState("");
  const [loading, setLoading] = useState(true);
  const [copied, setCopied] = useState("");
  const [busy, setBusy] = useState("");

  async function load() {
    try {
      const j = await webhooksApi.list();
      setRows(asPublic(j.webhooks));
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
    setBusy("create");
    try {
      const created = asCreated(await webhooksApi.create());
      setLast(created);
      setCopied("");
      setOk("");
      setErr("");
      await load();
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  async function rotate(id: string) {
    setBusy("rotate:" + id);
    try {
      const created = asCreated(await webhooksApi.rotate(id));
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

  async function revoke(id: string) {
    setBusy("revoke:" + id);
    try {
      await webhooksApi.revoke(id);
      setOk(t("webhooks.revokedOk"));
      setErr("");
      if (last?.id === id) {
        setLast(null);
      }
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
      setLast({
        ...last,
        token: kind === "token" ? undefined : last.token,
        hmac_key: kind === "hmac" ? undefined : last.hmac_key,
      });
      setCopied(t("webhooks.copied"));
    } catch (e) {
      setErr(formatPublicError(e));
    }
  }

  return (
    <div style={{ padding: "14px 22px 40px", display: "flex", flexDirection: "column", gap: 14 }}>
      <SectionHeader
        icon="hook"
        title={t("webhooks.title")}
        description={t("webhooks.desc")}
        actions={
          <>
            <Button icon="refresh" iconGesture onClick={() => void load()}>
              {t("common.refresh")}
            </Button>
            <Button variant="primary" icon="plus" disabled={busy === "create"} onClick={() => void create()}>
              {t("webhooks.create")}
            </Button>
          </>
        }
      />
      {err ? <StatusLine kind="error">{err}</StatusLine> : null}
      {ok && !err ? (
        <p role="status" style={{ margin: 0, fontSize: 12.5, color: "var(--green)" }}>
          {ok}
        </p>
      ) : null}
      <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("webhooks.noSecrets")}</p>
      <Card>
        <CardHeader icon="hook" title={t("webhooks.list")} meta={t("webhooks.meta", { n: rows.length })} />
        <TableScroll>
        <div style={{ display: "flex", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)", gap: 8 }}>
          <span style={{ flex: 2 }}>{t("webhooks.col.id")}</span>
          <span style={{ flex: 1 }}>{t("webhooks.col.prefix")}</span>
          <span style={{ width: 90 }}>{t("webhooks.col.hmacReq")}</span>
          <span style={{ width: 160 }}>{t("webhooks.col.actions")}</span>
        </div>
        {rows.map((w) => (
          <div key={w.id} style={{ display: "flex", padding: "11px 16px", fontSize: 12.5, borderBottom: "1px solid var(--border-soft)", gap: 8, alignItems: "center" }}>
            <code style={{ flex: 2, fontSize: 12 }}>{w.id}</code>
            <code style={{ flex: 1, fontSize: 12 }}>{w.token_prefix}</code>
            <span style={{ width: 90 }}>
              {w.require_hmac ? <Badge tone="warning">{t("webhooks.requireHmac")}</Badge> : <Badge tone="neutral">{w.kind || "llm"}</Badge>}
            </span>
            <span style={{ width: 160, display: "flex", gap: 6 }}>
              <Button disabled={busy.startsWith("rotate")} onClick={() => void rotate(w.id)} style={{ padding: "4px 10px" }}>
                {t("webhooks.rotate")}
              </Button>
              <Button disabled={busy.startsWith("revoke")} onClick={() => void revoke(w.id)} style={{ padding: "4px 10px" }}>
                {t("webhooks.revoke")}
              </Button>
            </span>
          </div>
        ))}
        {loading ? <StatusLine kind="loading" /> : rows.length === 0 ? <EmptyState>{t("webhooks.empty")}</EmptyState> : null}
        </TableScroll>
      </Card>
      <Card>
        <CardHeader icon="lock" title={t("webhooks.last")} />
        {!last ? (
          <EmptyState>{t("webhooks.lastEmpty")}</EmptyState>
        ) : (
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
          </div>
        )}
      </Card>
    </div>
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
