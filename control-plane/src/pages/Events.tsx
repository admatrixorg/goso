import { useEffect, useState } from "react";
import { api, type GatewayEvent } from "../api/client";
import { useI18n } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { SectionHeader } from "../ui/SectionHeader";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

function kindTone(k: string): "positive" | "warning" | "critical" | "neutral" | "accent" {
  if (k.includes("error") || k.includes("fail")) return "critical";
  if (k.includes("success") || k.includes("ok")) return "positive";
  if (k.includes("pending") || k.includes("approval")) return "warning";
  if (k.includes("attempt")) return "accent";
  return "neutral";
}

export function EventsPage() {
  const { t } = useI18n();
  const [events, setEvents] = useState<GatewayEvent[]>([]);
  const [kind, setKind] = useState("");
  const [connector, setConnector] = useState("");
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(true);

  async function load() {
    try {
      const j = await api.listEvents({
        kind: kind || undefined,
        connector: connector || undefined,
        limit: 100,
      });
      setEvents(j.events ?? []);
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

  return (
    <div style={{ padding: "14px 22px 40px", display: "flex", flexDirection: "column", gap: 14 }}>
      <SectionHeader
        icon="history"
        title={t("events.title")}
        description={t("events.desc")}
        actions={
          <Button icon="refresh" iconGesture onClick={() => void load()}>
            {t("common.refresh")}
          </Button>
        }
      />
      {err ? <StatusLine kind="error">{err}</StatusLine> : null}
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
        <input className="z-field" placeholder="kind" value={kind} onChange={(e) => setKind(e.target.value)} />
        <input className="z-field" placeholder="connector" value={connector} onChange={(e) => setConnector(e.target.value)} />
      </div>
      <Card>
        <CardHeader icon="pulse" title={t("events.list")} meta={t("events.meta", { n: events.length })} />
        <TableScroll>
        <div style={{ display: "flex", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)" }}>
          <span style={{ flex: 1.4 }}>{t("events.col.ts")}</span>
          <span style={{ flex: 1.1 }}>{t("events.col.kind")}</span>
          <span style={{ flex: 1.1 }}>{t("events.col.connector")}</span>
          <span style={{ flex: 1 }}>{t("events.col.tool")}</span>
          <span style={{ flex: 2.2 }}>{t("events.col.summary")}</span>
        </div>
        {events.map((e, i) => (
          <div key={e.trace_id + e.kind + i} style={{ display: "flex", alignItems: "center", padding: "11px 16px", fontSize: 12.5, borderBottom: "1px solid var(--border-soft)" }}>
            <span style={{ flex: 1.4, color: "var(--text-3)", fontVariantNumeric: "tabular-nums" }}>{e.ts}</span>
            <span style={{ flex: 1.1 }}>
              <Badge tone={kindTone(e.kind)}>{e.kind}</Badge>
            </span>
            <span style={{ flex: 1.1 }}>{e.connector}</span>
            <span style={{ flex: 1, color: "var(--text-2)" }}>{e.tool}</span>
            <span style={{ flex: 2.2, color: "var(--text-2)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{e.summary}</span>
          </div>
        ))}
        {loading ? <StatusLine kind="loading" /> : events.length === 0 ? <EmptyState>{t("events.empty")}</EmptyState> : null}
        </TableScroll>
      </Card>
    </div>
  );
}
