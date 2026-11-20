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
import { Button } from "../ui/Button";
import { Card, CardHeader } from "../ui/Card";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { DataTable, THead, Th, Td, Tr } from "../ui/DataTable";
import { EmptyState } from "../ui/EmptyState";
import { PageChrome } from "../ui/PageChrome";
import { PageStatus } from "../ui/PageStatus";
import { SearchInput } from "../ui/SearchInput";
import { StatusBadge } from "../ui/StatusBadge";
import { StatusLine, formatPublicError, redactPublicText } from "../ui/StatusLine";
import { fieldClass, hintClass, labelClass, selectClass } from "../ui/field";

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
  const [clearOpen, setClearOpen] = useState(false);
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
    setClearOpen(false);
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
      icon="cpu"
      title={t("providers.title")}
      description={t("providers.desc")}
      primary={
        <Button variant="default" icon="plus" disabled={blocked} onClick={resetForm}>
          {t("providers.add")}
        </Button>
      }
      refresh={
        <Button variant="outline" icon="refresh" iconGesture onClick={() => void load()}>
          {t("common.refresh")}
        </Button>
      }
      filters={
        <>
          <SearchInput value={query} onChange={setQuery} placeholder={t("providers.search")} aria-label={t("providers.search")} />
          <select
            className={selectClass}
            aria-label={t("providers.filterType")}
            value={typeFilter}
            disabled={blocked}
            onChange={(e) => setTypeFilter(e.target.value)}
          >
            <option value="">{t("providers.filterTypeAll")}</option>
            {typeOptions.map((typ) => (
              <option key={typ} value={typ}>
                {typ}
              </option>
            ))}
          </select>
          <select
            className={selectClass}
            aria-label={t("providers.filterSource")}
            value={sourceFilter}
            disabled={blocked}
            onChange={(e) => setSourceFilter(e.target.value as ProviderSourceFilter)}
          >
            <option value="">{t("providers.filterSourceAll")}</option>
            <option value="env">{t("providers.source.env")}</option>
            <option value="sqlite">{t("providers.source.sqlite")}</option>
          </select>
          <select
            className={selectClass}
            aria-label={t("providers.filterEnabled")}
            value={enabledFilter}
            disabled={blocked}
            onChange={(e) => setEnabledFilter(e.target.value as ProviderEnabledFilter)}
          >
            <option value="">{t("providers.filterEnabledAll")}</option>
            <option value="on">{t("providers.status.on")}</option>
            <option value="off">{t("providers.status.off")}</option>
          </select>
        </>
      }
    >
      <p className="text-sm text-muted-foreground">{t("providers.noSecrets")}</p>
      <PageStatus kind={state.kind} errorText={err ? formatPublicError(err) : ""} staleAt={formatStaleAt(loadedAt, locale)} onReload={() => void load()} />
      {formErr ? <StatusLine kind="error">{formErr}</StatusLine> : null}
      {ok ? (
        <p role="status" className="text-sm text-success">
          {ok}
        </p>
      ) : null}
      {state.showEmpty ? <EmptyState icon="cpu" title={t("providers.empty")} data-page-state="empty" /> : null}
      {filteredEmpty ? <EmptyState icon="search" title={t("providers.filterEmpty")} data-page-state="filtered_empty" /> : null}
      {state.showItems && visible.length > 0 ? (
        <>
          <div className="flex items-center justify-between">
            <h2 className="text-sm font-semibold">{t("providers.list")}</h2>
            <span className="text-xs text-muted-foreground">{metaN == null ? "—" : t("providers.meta", { n: metaN })}</span>
          </div>
          <DataTable minWidth="900px">
            <THead>
              <Th>{t("providers.col.name")}</Th>
              <Th>{t("providers.col.type")}</Th>
              <Th>{t("providers.col.baseUrl")}</Th>
              <Th>{t("providers.col.model")}</Th>
              <Th>{t("providers.col.keySet")}</Th>
              <Th>{t("providers.col.source")}</Th>
              <Th>{t("providers.col.enabled")}</Th>
            </THead>
            <tbody>
              {visible.map((row) => (
                <Tr key={row.name} selected={selected === row.name} onClick={() => pick(row)}>
                  <Td className="font-semibold">{row.name}</Td>
                  <Td>{row.type}</Td>
                  <Td className="max-w-[220px] truncate text-muted-foreground">{row.base_url || "—"}</Td>
                  <Td className="max-w-[180px] truncate">{row.model || "—"}</Td>
                  <Td>
                    <StatusBadge
                      status={row.key_set ? "success" : "default"}
                      label={row.key_set ? t("providers.keySet") : t("providers.keyMissing")}
                    />
                  </Td>
                  <Td className="text-muted-foreground">{row.source === "env" ? t("providers.source.env") : t("providers.source.sqlite")}</Td>
                  <Td>
                    <StatusBadge
                      status={isProviderEnabled(row) ? "success" : "warning"}
                      label={isProviderEnabled(row) ? t("providers.status.on") : t("providers.status.off")}
                    />
                  </Td>
                </Tr>
              ))}
            </tbody>
          </DataTable>
        </>
      ) : null}
      {!blocked ? (
        <Card>
          <CardHeader icon="bolt" title={editing ? t("providers.edit") : t("providers.add")} />
          <div className="flex flex-col gap-3 p-4">
            {envLocked ? <p className="text-sm text-muted-foreground">{t("providers.envLocked")}</p> : null}
            <label className={labelClass}>
              {t("providers.name")}
              <input
                className={fieldClass + " mt-1"}
                value={form.name}
                disabled={editing}
                autoComplete="off"
                onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
              />
            </label>
            <label className={labelClass}>
              {t("providers.type")}
              <select
                className={selectClass + " mt-1 w-full"}
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
            <label className={labelClass}>
              {t("providers.baseUrl")}
              <input
                className={fieldClass + " mt-1"}
                value={form.base_url}
                disabled={envLocked}
                autoComplete="off"
                onChange={(e) => setForm((f) => ({ ...f, base_url: e.target.value }))}
              />
            </label>
            <label className={labelClass}>
              {t("providers.model")}
              <input
                className={fieldClass + " mt-1"}
                value={form.model}
                disabled={envLocked}
                autoComplete="off"
                onChange={(e) => setForm((f) => ({ ...f, model: e.target.value }))}
              />
            </label>
            <label className={labelClass}>
              {t("providers.apiKey")}
              <input
                className={fieldClass + " mt-1"}
                type="password"
                autoComplete="new-password"
                value={form.api_key}
                disabled={envLocked}
                onChange={(e) => setForm((f) => ({ ...f, api_key: e.target.value }))}
              />
            </label>
            <p className={hintClass}>{t("providers.apiKeyHint")}</p>
            <label className={labelClass + " flex items-center gap-2"}>
              <input
                type="checkbox"
                checked={form.enabled}
                disabled={envLocked}
                onChange={(e) => setForm((f) => ({ ...f, enabled: e.target.checked }))}
              />
              {t("providers.enabled")}
            </label>
            <div className="flex flex-wrap items-center gap-2">
              <Button variant="default" disabled={saving || envLocked} onClick={() => void save()}>
                {editing ? (form.api_key.trim() ? t("providers.rotate") : t("common.save")) : t("common.create")}
              </Button>
              <Button variant="outline" onClick={resetForm}>
                {t("providers.new")}
              </Button>
              {editing && current && canClearProviderKey(current) ? (
                <Button variant="ghost" className="text-destructive" disabled={clearing || envLocked} onClick={() => setClearOpen(true)}>
                  {t("providers.clear")}
                </Button>
              ) : null}
              <select className={selectClass} value={testKind} onChange={(e) => setTestKind(e.target.value as "models" | "chat")}>
                <option value="models">{t("providers.kind.models")}</option>
                <option value="chat">{t("providers.kind.chat")}</option>
              </select>
              <Button variant="outline" disabled={testing} onClick={() => void runTest()}>
                {testing ? t("providers.testing") : t("providers.test")}
              </Button>
            </div>
            {testing ? <StatusLine kind="loading">{t("providers.testing")}</StatusLine> : null}
            {testView ? (
              <div className="flex flex-col gap-1.5 text-sm">
                <div className="flex flex-wrap items-center gap-2">
                  <StatusBadge status={testView.ok ? "success" : "error"} label={testView.ok ? t("providers.testOk") : t("providers.testFail")} />
                  <span className="tabular-nums text-muted-foreground">{t("providers.latency", { ms: testView.latency_ms })}</span>
                </div>
                {testView.models.length > 0 ? (
                  <p className="text-sm text-foreground">{t("providers.modelsFound", { list: testView.models.join(", ") })}</p>
                ) : null}
                {testView.reply ? (
                  <p className="text-sm text-foreground">{t("providers.reply", { text: redactPublicText(testView.reply) })}</p>
                ) : null}
                {testView.error ? <StatusLine kind="error">{redactPublicText(testView.error)}</StatusLine> : null}
              </div>
            ) : null}
          </div>
        </Card>
      ) : null}
      <ConfirmDialog
        open={clearOpen}
        onOpenChange={setClearOpen}
        title={t("providers.confirmClear", { name: current?.name ?? "" })}
        confirmLabel={t("providers.clear")}
        variant="destructive"
        onConfirm={() => void clearKey()}
        loading={clearing}
      />
    </PageChrome>
  );
}
