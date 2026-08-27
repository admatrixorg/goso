import { useEffect, useState } from "react";
import { providersApi } from "../api/providers";
import { useI18n } from "../i18n";
import { Button } from "../ui/Button";
import { Card, CardHeader } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { SectionHeader } from "../ui/SectionHeader";

export function ProvidersPage() {
  const { t } = useI18n();
  const [names, setNames] = useState<string[]>([]);
  const [err, setErr] = useState("");

  async function load() {
    try {
      const j = await providersApi.list();
      setNames((j.providers ?? []).filter((n) => typeof n === "string"));
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
        icon="bolt"
        title={t("providers.title")}
        description={t("providers.desc")}
        actions={
          <Button icon="refresh" iconGesture onClick={() => void load()}>
            {t("common.refresh")}
          </Button>
        }
      />
      {err ? <p style={{ color: "var(--red)", fontSize: 12.5, margin: 0 }}>{err}</p> : null}
      <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("providers.noSecrets")}</p>
      <Card>
        <CardHeader icon="bolt" title={t("providers.list")} meta={t("providers.meta", { n: names.length })} />
        <div style={{ display: "flex", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)" }}>
          <span style={{ flex: 1 }}>{t("providers.col.name")}</span>
        </div>
        {names.map((n) => (
          <div key={n} style={{ display: "flex", padding: "11px 16px", fontSize: 12.5, borderBottom: "1px solid var(--border-soft)" }}>
            <span style={{ flex: 1, fontWeight: 600 }}>{n}</span>
          </div>
        ))}
        {names.length === 0 ? <EmptyState>{t("providers.empty")}</EmptyState> : null}
      </Card>
    </div>
  );
}
