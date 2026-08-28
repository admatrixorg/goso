import { useEffect, useState } from "react";
import { providersApi, type ProviderInfo, type ProviderTestResult } from "../api/providers";
import { useI18n } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { SectionHeader } from "../ui/SectionHeader";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

const TYPES = ["openai-compat", "anthropic", "echo", "router9"] as const;

const emptyForm = { name: "", type: "openai-compat", base_url: "", model: "", api_key: "" };

export function ProvidersPage() {
  const { t } = useI18n();
  const [rows, setRows] = useState<ProviderInfo[]>([]);
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [selected, setSelected] = useState("");
  const [form, setForm] = useState(emptyForm);
  const [testKind, setTestKind] = useState<"models" | "chat">("models");
  const [testing, setTesting] = useState(false);
  const [testRaw, setTestRaw] = useState("");

  const current = rows.find((r) => r.name === selected);
  const envLocked = current?.source === "env";
  const editing = Boolean(selected);

  async function load() {
    setLoading(true);
    try {
      const j = await providersApi.list();
      const list = (j.providers ?? []).filter((p) => p && typeof p === "object" && typeof p.name === "string");
      setRows(list);
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

  function pick(row: ProviderInfo) {
    setSelected(row.name);
    setForm({ name: row.name, type: row.type || "openai-compat", base_url: row.base_url || "", model: row.model || "", api_key: "" });
    setTestRaw("");
    setErr("");
  }

  function resetForm() {
    setSelected("");
    setForm(emptyForm);
    setTestRaw("");
    setErr("");
  }

  async function save() {
    setSaving(true);
    setErr("");
    try {
      if (editing) {
        await providersApi.patch(selected, {
          type: form.type,
          base_url: form.base_url,
          model: form.model,
          api_key: form.api_key,
        });
        setForm((f) => ({ ...f, api_key: "" }));
      } else {
        const created = await providersApi.create({
          name: form.name.trim(),
          type: form.type,
          base_url: form.base_url,
          model: form.model,
          api_key: form.api_key,
        });
        setSelected(created.name);
        setForm({
          name: created.name,
          type: created.type || form.type,
          base_url: created.base_url || form.base_url,
          model: created.model || form.model,
          api_key: "",
        });
      }
      await load();
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setSaving(false);
    }
  }

  async function runTest() {
    const name = editing ? selected : form.name.trim();
    if (!name) {
      setErr(t("providers.needName"));
      return;
    }
    setTesting(true);
    setErr("");
    try {
      const result: ProviderTestResult = await providersApi.test(name, testKind);
      setTestRaw(JSON.stringify(result, null, 2));
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setTesting(false);
    }
  }

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
      {err ? <StatusLine kind="error">{err}</StatusLine> : null}
      <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("providers.noSecrets")}</p>
      <Card>
        <CardHeader icon="bolt" title={t("providers.list")} meta={t("providers.meta", { n: rows.length })} />
        <TableScroll>
        <div style={{ display: "flex", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)", gap: 8 }}>
          <span style={{ flex: 1.2 }}>{t("providers.col.name")}</span>
          <span style={{ flex: 1 }}>{t("providers.col.type")}</span>
          <span style={{ flex: 1.6 }}>{t("providers.col.baseUrl")}</span>
          <span style={{ flex: 1.2 }}>{t("providers.col.model")}</span>
          <span style={{ width: 72 }}>{t("providers.col.keySet")}</span>
          <span style={{ width: 72 }}>{t("providers.col.source")}</span>
        </div>
        {rows.map((row) => {
          const on = selected === row.name;
          return (
            <button
              key={row.name}
              type="button"
              onClick={() => pick(row)}
              style={{
                display: "flex",
                width: "100%",
                textAlign: "left",
                padding: "11px 16px",
                fontSize: 12.5,
                border: "none",
                borderBottom: "1px solid var(--border-soft)",
                background: on ? "var(--accent-soft)" : "transparent",
                color: "var(--text)",
                gap: 8,
                cursor: "pointer",
                alignItems: "center",
              }}
            >
              <span style={{ flex: 1.2, fontWeight: 600 }}>{row.name}</span>
              <span style={{ flex: 1 }}>{row.type}</span>
              <span style={{ flex: 1.6, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", color: "var(--text-2)" }}>{row.base_url || "—"}</span>
              <span style={{ flex: 1.2, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{row.model || "—"}</span>
              <span style={{ width: 72 }}>
                <Badge tone={row.key_set ? "positive" : "neutral"}>{row.key_set ? t("common.yes") : t("common.no")}</Badge>
              </span>
              <span style={{ width: 72, color: "var(--text-3)" }}>{row.source}</span>
            </button>
          );
        })}
        {loading ? <StatusLine kind="loading" /> : rows.length === 0 ? <EmptyState>{t("providers.empty")}</EmptyState> : null}
        </TableScroll>
      </Card>
      <Card>
        <CardHeader icon="bolt" title={editing ? t("providers.edit") : t("providers.add")} />
        <div style={{ padding: 14, display: "flex", flexDirection: "column", gap: 10 }}>
          {envLocked ? <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("providers.envLocked")}</p> : null}
          <label style={{ fontSize: 12, color: "var(--text-2)" }}>
            {t("providers.name")}
            <input
              className="z-field"
              style={{ display: "block", width: "100%", marginTop: 4 }}
              value={form.name}
              disabled={editing}
              autoComplete="off"
              onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
            />
          </label>
          <label style={{ fontSize: 12, color: "var(--text-2)" }}>
            {t("providers.type")}
            <select
              className="z-field"
              style={{ display: "block", width: "100%", marginTop: 4 }}
              value={form.type}
              disabled={envLocked}
              onChange={(e) => setForm((f) => ({ ...f, type: e.target.value }))}
            >
              {TYPES.map((typ) => (
                <option key={typ} value={typ}>
                  {typ}
                </option>
              ))}
            </select>
          </label>
          <label style={{ fontSize: 12, color: "var(--text-2)" }}>
            {t("providers.baseUrl")}
            <input
              className="z-field"
              style={{ display: "block", width: "100%", marginTop: 4 }}
              value={form.base_url}
              disabled={envLocked}
              autoComplete="off"
              onChange={(e) => setForm((f) => ({ ...f, base_url: e.target.value }))}
            />
          </label>
          <label style={{ fontSize: 12, color: "var(--text-2)" }}>
            {t("providers.model")}
            <input
              className="z-field"
              style={{ display: "block", width: "100%", marginTop: 4 }}
              value={form.model}
              disabled={envLocked}
              autoComplete="off"
              onChange={(e) => setForm((f) => ({ ...f, model: e.target.value }))}
            />
          </label>
          <label style={{ fontSize: 12, color: "var(--text-2)" }}>
            {t("providers.apiKey")}
            <input
              className="z-field"
              type="password"
              autoComplete="new-password"
              style={{ display: "block", width: "100%", marginTop: 4 }}
              value={form.api_key}
              disabled={envLocked}
              onChange={(e) => setForm((f) => ({ ...f, api_key: e.target.value }))}
            />
          </label>
          <p style={{ margin: 0, fontSize: 12, color: "var(--text-3)" }}>{t("providers.apiKeyHint")}</p>
          <div style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "center" }}>
            <Button variant="primary" disabled={saving || envLocked} onClick={() => void save()}>
              {editing ? t("common.save") : t("common.create")}
            </Button>
            <Button onClick={resetForm}>{t("providers.new")}</Button>
            <select className="z-field" value={testKind} onChange={(e) => setTestKind(e.target.value as "models" | "chat")}>
              <option value="models">{t("providers.kind.models")}</option>
              <option value="chat">{t("providers.kind.chat")}</option>
            </select>
            <Button disabled={testing} onClick={() => void runTest()}>
              {testing ? t("providers.testing") : t("providers.test")}
            </Button>
          </div>
          {testing ? <StatusLine kind="loading">{t("providers.testing")}</StatusLine> : null}
          {testRaw ? (
            <pre
              style={{
                margin: 0,
                padding: 12,
                fontSize: 12,
                lineHeight: 1.4,
                overflow: "auto",
                background: "var(--surface-2)",
                borderRadius: 8,
                border: "1px solid var(--border-soft)",
              }}
            >
              {testRaw}
            </pre>
          ) : null}
        </div>
      </Card>
    </div>
  );
}
