import { useEffect, useMemo, useState } from "react";
import {
  canClearProviderKey,
  filterProviders,
  formatProviderTest,
  isEnvOwned,
  isProviderEnabled,
  PROVIDER_TYPES,
  providersApi,
  providerWriteBody,
  uniqueProviderTypes,
  type ProviderEnabledFilter,
  type ProviderInfo,
  type ProviderSourceFilter,
  type ProviderTestView,
} from "../api/providers";
import { classifyPageState, formatStaleAt, inventoryBlocksMutation, isFilteredEmpty, listMetaCount } from "../api/page-state";
import { useI18n } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { PageChrome } from "../ui/PageChrome";
import { PageStatus } from "../ui/PageStatus";
import { StatusLine, formatPublicError, redactPublicText } from "../ui/StatusLine";

const emptyForm = { name: "", type: "openai-compat", base_url: "", model: "", api_key: "", enabled: true };

export function ProvidersPage() {
  const { t, locale } = useI18n();
  const [rows, setRows] = useState<ProviderInfo[]>([]);
  const [err, setErr] = useState<unknown>(null);
  const [formErr, setFormErr] = useState("");
  const [ok, setOk] = useState("");
  const [loading, setLoading] = useState(true);
  const [loaded, setLoaded] = useState(false);
  const [loadedAt, setLoadedAt] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [clearing, setClearing] = useState(false);
  const [selected, setSelected] = useState("");
  const [form, setForm] = useState(emptyForm);
  const [testKind, setTestKind] = useState<"models" | "chat">("models");
  const [testing, setTesting] = useState(false);
  const [testView, setTestView] = useState<ProviderTestView | null>(null);
  const [query, setQuery] = useState("");
  const [typeFilter, setTypeFilter] = useState("");
  const [sourceFilter, setSourceFilter] = useState<ProviderSourceFilter>("");
  const [enabledFilter, setEnabledFilter] = useState<ProviderEnabledFilter>("");

  const state = classifyPageState({
    loading,
    loaded,
    error: err,
    itemCount: rows.length,
    keepStale: loaded && rows.length > 0,
  });
  const blocked = inventoryBlocksMutation(state.kind);
  const current = rows.find((r) => r.name === selected);
  const envLocked = current ? isEnvOwned(current) : false;
  const editing = Boolean(selected);
  const typeOptions = useMemo(() => uniqueProviderTypes(rows), [rows]);
  const visible = useMemo(
    () => filterProviders(state.showItems ? rows : [], { query, type: typeFilter, source: sourceFilter, enabled: enabledFilter }),
    [rows, query, typeFilter, sourceFilter, enabledFilter, state.showItems],
  );
  const filteredEmpty = isFilteredEmpty(state, rows.length, visible.length);
  const metaN = listMetaCount(state.kind, rows.length);
  const formLocked = blocked || envLocked;

  async function load() {
    setLoading(true);
    try {
      const j = await providersApi.list();
      const list = (j.providers ?? []).filter((p) => p && typeof p === "object" && typeof p.name === "string");
      setRows(list);
      setLoaded(true);
      setLoadedAt(new Date().toISOString());
      setErr(null);
    } catch (e) {
      setErr(e);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  function pick(row: ProviderInfo) {
    if (blocked) return;
    setSelected(row.name);
    setForm({
      name: row.name,
      type: row.type || "openai-compat",
      base_url: row.base_url || "",
      model: row.model || "",
      api_key: "",
      enabled: isProviderEnabled(row),
    });
    setTestView(null);
    setFormErr("");
    setOk("");
  }

  function resetForm() {
    if (blocked) return;
    setSelected("");
    setForm(emptyForm);
    setTestView(null);
    setFormErr("");
    setOk("");
  }

  async function save() {
    if (formLocked) return;
    setSaving(true);
    setFormErr("");
    setOk("");
    try {
      const body = providerWriteBody(form);
      if (editing) {
        await providersApi.patch(selected, body);
        setForm((f) => ({ ...f, api_key: "" }));
      } else {
        const created = await providersApi.create(body);
        setSelected(created.name);
        setForm({
          name: created.name,
          type: created.type || form.type,
          base_url: created.base_url || form.base_url,
          model: created.model || form.model,
          api_key: "",
          enabled: isProviderEnabled(created),
        });
      }
      await load();
    } catch (e) {
      setFormErr(formatPublicError(e));
    } finally {
      setSaving(false);
    }
  }

  async function clearKey() {
    if (!current || !canClearProviderKey(current) || formLocked) return;
    if (!window.confirm(t("providers.confirmClear", { name: current.name }))) return;
    setClearing(true);
    setFormErr("");
    setOk("");
    try {
      await providersApi.clearKey(current.name);
      setForm((f) => ({ ...f, api_key: "" }));
      setOk(t("providers.cleared"));
      await load();
    } catch (e) {
      setFormErr(formatPublicError(e));
    } finally {
      setClearing(false);
    }
  }

  async function runTest() {
    if (blocked) return;
    const name = editing ? selected : form.name.trim();
    if (!name) {
      setFormErr(t("providers.needName"));
      return;
    }
    setTesting(true);
    setFormErr("");
    setOk("");
    try {
      const result = await providersApi.test(name, testKind);
      setTestView(formatProviderTest(result));
    } catch (e) {
      setTestView(null);
      setFormErr(formatPublicError(e));
    } finally {
      setTesting(false);
    }
  }

  return (
    <PageChrome
      icon="bolt"
      title={t("providers.title")}
      description={t("providers.desc")}
      primary={
        <Button variant="primary" icon="plus" disabled={blocked} onClick={resetForm}>
          {t("providers.add")}
        </Button>
      }
      refresh={
        <Button icon="refresh" iconGesture onClick={() => void load()}>
          {t("common.refresh")}
        </Button>
      }
      filters={
        <>
          <input
            className="z-field"
            style={{ flex: 1, minWidth: 160 }}
            value={query}
            disabled={blocked}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={t("providers.search")}
            aria-label={t("providers.search")}
            autoComplete="off"
          />
          <select
            className="z-field"
            aria-label={t("providers.filterType")}
            value={typeFilter}
            disabled={blocked}
            onChange={(e) => setTypeFilter(e.target.value)}
            style={{ minWidth: 140 }}
          >
            <option value="">{t("providers.filterTypeAll")}</option>
            {typeOptions.map((typ) => (
              <option key={typ} value={typ}>
                {typ}
              </option>
            ))}
          </select>
          <select
            className="z-field"
            aria-label={t("providers.filterSource")}
            value={sourceFilter}
            disabled={blocked}
            onChange={(e) => setSourceFilter(e.target.value as ProviderSourceFilter)}
            style={{ minWidth: 120 }}
          >
            <option value="">{t("providers.filterSourceAll")}</option>
            <option value="env">{t("providers.source.env")}</option>
            <option value="sqlite">{t("providers.source.sqlite")}</option>
          </select>
          <select
            className="z-field"
            aria-label={t("providers.filterEnabled")}
            value={enabledFilter}
            disabled={blocked}
            onChange={(e) => setEnabledFilter(e.target.value as ProviderEnabledFilter)}
            style={{ minWidth: 120 }}
          >
            <option value="">{t("providers.filterEnabledAll")}</option>
            <option value="on">{t("providers.status.on")}</option>
            <option value="off">{t("providers.status.off")}</option>
          </select>
        </>
      }
    >
      <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("providers.noSecrets")}</p>
      <PageStatus kind={state.kind} errorText={err ? formatPublicError(err) : ""} staleAt={formatStaleAt(loadedAt, locale)} onReload={() => void load()} />
      {formErr ? <StatusLine kind="error">{formErr}</StatusLine> : null}
      {ok ? <p style={{ margin: 0, fontSize: 12.5, color: "var(--green)" }}>{ok}</p> : null}
      <Card>
        <CardHeader icon="bolt" title={t("providers.list")} meta={metaN == null ? "—" : t("providers.meta", { n: metaN })} />
        <TableScroll>
        <div style={{ display: "flex", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)", gap: 8 }}>
          <span style={{ flex: 1.2 }}>{t("providers.col.name")}</span>
          <span style={{ flex: 1 }}>{t("providers.col.type")}</span>
          <span style={{ flex: 1.6 }}>{t("providers.col.baseUrl")}</span>
          <span style={{ flex: 1.2 }}>{t("providers.col.model")}</span>
          <span style={{ width: 88 }}>{t("providers.col.keySet")}</span>
          <span style={{ width: 72 }}>{t("providers.col.source")}</span>
          <span style={{ width: 56 }}>{t("providers.col.enabled")}</span>
        </div>
        {visible.map((row) => {
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
                cursor: blocked ? "default" : "pointer",
                alignItems: "center",
              }}
            >
              <span style={{ flex: 1.2, fontWeight: 600 }}>{row.name}</span>
              <span style={{ flex: 1 }}>{row.type}</span>
              <span style={{ flex: 1.6, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", color: "var(--text-2)" }}>{row.base_url || "—"}</span>
              <span style={{ flex: 1.2, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{row.model || "—"}</span>
              <span style={{ width: 88 }}>
                <Badge tone={row.key_set ? "positive" : "neutral"}>{row.key_set ? t("providers.keySet") : t("providers.keyMissing")}</Badge>
              </span>
              <span style={{ width: 72, color: "var(--text-3)" }}>{row.source === "env" ? t("providers.source.env") : t("providers.source.sqlite")}</span>
              <span style={{ width: 56 }}>
                <Badge tone={isProviderEnabled(row) ? "positive" : "warning"}>
                  {isProviderEnabled(row) ? t("providers.status.on") : t("providers.status.off")}
                </Badge>
              </span>
            </button>
          );
        })}
        {state.showEmpty ? <EmptyState data-page-state="empty">{t("providers.empty")}</EmptyState> : null}
        {filteredEmpty ? <EmptyState data-page-state="filtered_empty">{t("providers.filterEmpty")}</EmptyState> : null}
        </TableScroll>
      </Card>
      {!blocked ? (
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
              {PROVIDER_TYPES.map((typ) => (
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
          <label style={{ fontSize: 12, color: "var(--text-2)", display: "flex", alignItems: "center", gap: 8 }}>
            <input
              type="checkbox"
              checked={form.enabled}
              disabled={envLocked}
              onChange={(e) => setForm((f) => ({ ...f, enabled: e.target.checked }))}
            />
            {t("providers.enabled")}
          </label>
          <div style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "center" }}>
            <Button variant="primary" disabled={saving || envLocked} onClick={() => void save()}>
              {editing ? (form.api_key.trim() ? t("providers.rotate") : t("common.save")) : t("common.create")}
            </Button>
            <Button onClick={resetForm}>{t("providers.new")}</Button>
            {editing && current && canClearProviderKey(current) ? (
              <Button disabled={clearing || envLocked} onClick={() => void clearKey()}>
                {t("providers.clear")}
              </Button>
            ) : null}
            <select className="z-field" value={testKind} onChange={(e) => setTestKind(e.target.value as "models" | "chat")}>
              <option value="models">{t("providers.kind.models")}</option>
              <option value="chat">{t("providers.kind.chat")}</option>
            </select>
            <Button disabled={testing} onClick={() => void runTest()}>
              {testing ? t("providers.testing") : t("providers.test")}
            </Button>
          </div>
          {testing ? <StatusLine kind="loading">{t("providers.testing")}</StatusLine> : null}
          {testView ? (
            <div style={{ display: "flex", flexDirection: "column", gap: 6, fontSize: 12.5 }}>
              <div style={{ display: "flex", gap: 8, alignItems: "center", flexWrap: "wrap" }}>
                <Badge tone={testView.ok ? "positive" : "critical"}>
                  {testView.ok ? t("providers.testOk") : t("providers.testFail")}
                </Badge>
                <span style={{ color: "var(--text-3)", fontVariantNumeric: "tabular-nums" }}>
                  {t("providers.latency", { ms: testView.latency_ms })}
                </span>
              </div>
              {testView.models.length > 0 ? (
                <p style={{ margin: 0, color: "var(--text-2)" }}>{t("providers.modelsFound", { list: testView.models.join(", ") })}</p>
              ) : null}
              {testView.reply ? (
                <p style={{ margin: 0, color: "var(--text-2)" }}>{t("providers.reply", { text: redactPublicText(testView.reply) })}</p>
              ) : null}
              {testView.error ? <StatusLine kind="error">{redactPublicText(testView.error)}</StatusLine> : null}
            </div>
          ) : null}
        </div>
      </Card>
      ) : null}
    </PageChrome>
  );
}
