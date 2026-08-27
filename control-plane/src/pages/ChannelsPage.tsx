import { useEffect, useState } from "react";
import { channelsApi, type ChannelRow } from "../api/channels";
import { useI18n } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { SectionHeader } from "../ui/SectionHeader";

export function ChannelsPage() {
  const { t } = useI18n();
  const [rows, setRows] = useState<ChannelRow[]>([]);
  const [err, setErr] = useState("");

  async function load() {
    try {
      const j = await channelsApi.list();
      const list = (j.channels ?? []).map((c) => ({
        name: typeof c?.name === "string" ? c.name : "",
        configured: c?.configured === true,
      }));
      setRows(list.filter((c) => c.name));
      setErr("");
    } catch (e) {
      setErr(String(e));
    }
  }
  useEffect(() => {
    void load();
  }, []);

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
      {err ? <p style={{ color: "var(--red)", fontSize: 12.5, margin: 0 }}>{err}</p> : null}
      <Card>
        <CardHeader icon="hook" title={t("channels.list")} meta={t("channels.meta", { n: rows.length })} />
        <div style={{ display: "flex", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)" }}>
          <span style={{ flex: 2 }}>{t("channels.col.name")}</span>
          <span style={{ flex: 1 }}>{t("channels.col.configured")}</span>
        </div>
        {rows.map((c) => (
          <div key={c.name} style={{ display: "flex", alignItems: "center", padding: "11px 16px", fontSize: 12.5, borderBottom: "1px solid var(--border-soft)" }}>
            <span style={{ flex: 2, fontWeight: 600 }}>{c.name}</span>
            <span style={{ flex: 1 }}>
              <Badge tone={c.configured ? "positive" : "neutral"}>{c.configured ? t("common.yes") : t("common.no")}</Badge>
            </span>
          </div>
        ))}
        {rows.length === 0 ? <EmptyState>{t("channels.empty")}</EmptyState> : null}
      </Card>
    </div>
  );
}
