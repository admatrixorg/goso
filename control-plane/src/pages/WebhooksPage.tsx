import { useState } from "react";
import { webhooksApi, type WebhookCreated } from "../api/webhooks";
import { useI18n } from "../i18n";
import { Button } from "../ui/Button";
import { Card, CardHeader } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { SectionHeader } from "../ui/SectionHeader";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

type LastCreate = {
  id: string;
  token_prefix: string;
  token?: string;
  hmac_key?: string;
};

function asCreated(j: WebhookCreated): LastCreate {
  return {
    id: typeof j.id === "string" ? j.id : "",
    token_prefix: typeof j.token_prefix === "string" ? j.token_prefix : "",
    token: typeof j.token === "string" ? j.token : undefined,
    hmac_key: typeof j.hmac_key === "string" ? j.hmac_key : undefined,
  };
}

export function WebhooksPage() {
  const { t } = useI18n();
  const [last, setLast] = useState<LastCreate | null>(null);
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(false);
  const [copied, setCopied] = useState("");

  async function create() {
    setLoading(true);
    try {
      const created = asCreated(await webhooksApi.create());
      setLast(created);
      setCopied("");
      setErr("");
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setLoading(false);
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
          <Button variant="primary" icon="plus" onClick={() => void create()}>
            {t("webhooks.create")}
          </Button>
        }
      />
      {err ? <StatusLine kind="error">{err}</StatusLine> : null}
      <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("webhooks.noList")}</p>
      <Card>
        <CardHeader icon="lock" title={t("webhooks.last")} />
        {loading ? <StatusLine kind="loading" /> : !last ? <EmptyState>{t("webhooks.empty")}</EmptyState> : (
          <div style={{ padding: "12px 16px", display: "flex", flexDirection: "column", gap: 10, fontSize: 12.5 }}>
            <p style={{ margin: 0, color: "var(--text-3)" }}>{t("webhooks.secretOnce")}</p>
            {copied ? <p style={{ margin: 0, color: "var(--green)" }}>{copied}</p> : null}
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
