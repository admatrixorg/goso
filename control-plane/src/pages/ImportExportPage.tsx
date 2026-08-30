import { useEffect, useMemo, useState } from "react";
import { impexpApi, type PortableJob, type Preview } from "../api/impexp";
import {
  catalogHasSecrets,
  downloadJSON,
  emptyCatalog,
  emptySelection,
  jobProgress,
  parseArchiveFile,
  publicHasSecrets,
  selectionCount,
  toggleId,
  type Catalog,
  type Conflict,
  type Manifest,
  type Selection,
} from "../api/impexp-ops";
import { classifyPageState, formatStaleAt, inventoryBlocksMutation, listMetaCount } from "../api/page-state";
import { useI18n, type MsgKey } from "../i18n";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { PageChrome } from "../ui/PageChrome";
import { PageStatus } from "../ui/PageStatus";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

type TabId = "teams" | "agents" | "skills" | "export" | "import";

const TAB_KEYS: Record<TabId, MsgKey> = {
  teams: "impexp.tab.teams",
  agents: "impexp.tab.agents",
  skills: "impexp.tab.skills",
  export: "impexp.tab.export",
  import: "impexp.tab.import",
};

const CONFLICTS: Conflict[] = ["skip", "overwrite", "rename"];

const CONFLICT_KEYS: Record<Conflict, MsgKey> = {
  skip: "impexp.conflict.skip",
  overwrite: "impexp.conflict.overwrite",
  rename: "impexp.conflict.rename",
};

function checkRow(id: string, label: string, checked: boolean, meta: string, onToggle: () => void, disabled = false) {
  return (
    <label
      key={id}
      style={{
        display: "flex",
        alignItems: "center",
        gap: 10,
        padding: "8px 16px",
        borderTop: "1px solid var(--border-soft)",
        fontSize: 13,
        cursor: disabled ? "default" : "pointer",
      }}
    >
      <input type="checkbox" checked={checked} disabled={disabled} onChange={onToggle} aria-label={label} />
      <span style={{ flex: 1, fontWeight: 600 }}>{label}</span>
      <span style={{ color: "var(--text-3)", fontSize: 12 }}>{meta}</span>
    </label>
  );
}

function ManifestCard({ manifest, title }: { manifest: Manifest; title: string }) {
  const rows = [
    ...manifest.teams.map((i) => ({ ...i, kind: "team" })),
    ...manifest.agents.map((i) => ({ ...i, kind: "agent" })),
    ...manifest.skills.map((i) => ({ ...i, kind: "skill" })),
    ...manifest.mcp.map((i) => ({ ...i, kind: "mcp" })),
  ];
  return (
    <Card>
      <CardHeader icon="doc" title={title} meta={String(rows.length)} />
      {rows.length === 0 ? <EmptyState>{"—"}</EmptyState> : null}
      <TableScroll>
        {rows.map((row, i) => (
          <div key={`${row.kind}-${row.name}-${i}`} style={{ display: "flex", gap: 12, padding: "8px 16px", borderTop: "1px solid var(--border-soft)", fontSize: 13 }}>
            <span style={{ width: 72, color: "var(--text-3)" }}>{row.kind}</span>
            <span style={{ flex: 1, fontWeight: 600 }}>{row.name}</span>
            <span style={{ color: "var(--text-3)" }}>{row.key || row.id || ""}</span>
          </div>
        ))}
      </TableScroll>
    </Card>
  );
}

function ReportCard({ job, t }: { job: PortableJob; t: (k: MsgKey, vars?: Record<string, string | number>) => string }) {
  const groups: { key: MsgKey; rows: { kind: string; name: string; detail?: string }[] }[] = [
    { key: "impexp.report.created", rows: job.report.created },
    { key: "impexp.report.skipped", rows: job.report.skipped },
    { key: "impexp.report.overwritten", rows: job.report.overwritten },
    { key: "impexp.report.renamed", rows: job.report.renamed },
    { key: "impexp.report.failed", rows: job.report.failed },
  ];
  return (
    <Card>
      <CardHeader icon="list" title={t("impexp.report")} meta={`${job.progress}%`} />
      <div style={{ padding: "0 16px 14px", display: "flex", flexDirection: "column", gap: 8 }}>
        <div style={{ height: 6, borderRadius: 99, background: "var(--border-soft)", overflow: "hidden" }}>
          <div style={{ width: `${jobProgress(job)}%`, height: "100%", background: "var(--accent)" }} />
        </div>
        {job.steps.map((s, i) => (
          <div key={`${s.name}-${i}`} style={{ fontSize: 12.5, color: "var(--text-2)" }}>
            {s.name}: {s.status}
            {s.detail ? ` · ${s.detail}` : ""}
          </div>
        ))}
        {groups.map((g) =>
          g.rows.length ? (
            <div key={g.key}>
              <div style={{ fontSize: 11, fontWeight: 700, color: "var(--text-3)", marginBottom: 4 }}>{t(g.key)}</div>
              {g.rows.map((r, i) => (
                <div key={`${r.kind}-${r.name}-${i}`} style={{ fontSize: 12.5 }}>
                  {r.kind} · {r.name}
                  {r.detail ? ` (${r.detail})` : ""}
                </div>
              ))}
            </div>
          ) : null,
        )}
        {job.report.credentials_needed.length ? (
          <p style={{ margin: 0, fontSize: 12.5, color: "var(--orange)" }}>
            {t("impexp.creds", { n: job.report.credentials_needed.length })}: {job.report.credentials_needed.map((c) => c.name).join(", ")}
          </p>
        ) : null}
      </div>
    </Card>
  );
}

export function ImportExportPage() {
  const { t, locale } = useI18n();
  const [tab, setTab] = useState<TabId>("teams");
  const [cat, setCat] = useState<Catalog>(emptyCatalog());
  const [sel, setSel] = useState<Selection>(emptySelection());
  const [loading, setLoading] = useState(true);
  const [loaded, setLoaded] = useState(false);
  const [loadedAt, setLoadedAt] = useState<string | null>(null);
  const [err, setErr] = useState<unknown>(null);
  const [actionErr, setActionErr] = useState("");
  const [ok, setOk] = useState("");
  const [busy, setBusy] = useState("");
  const [exportJob, setExportJob] = useState<PortableJob | null>(null);
  const [preview, setPreview] = useState<Preview | null>(null);
  const [importJob, setImportJob] = useState<PortableJob | null>(null);
  const [conflict, setConflict] = useState<Conflict>("skip");
  const [confirmImport, setConfirmImport] = useState(false);

  const catalogCount = cat.teams.length + cat.agents.length + cat.skills.length + cat.mcp.length;
  const state = classifyPageState({
    loading,
    loaded,
    error: err,
    itemCount: catalogCount,
    keepStale: loaded && catalogCount > 0,
  });
  const blocked = inventoryBlocksMutation(state.kind);
  const shownCat = state.showItems ? cat : emptyCatalog();
  const teamsMeta = listMetaCount(state.kind, cat.teams.length);
  const agentsMeta = listMetaCount(state.kind, cat.agents.length);
  const skillsMeta = listMetaCount(state.kind, cat.skills.length);
  const mcpMeta = listMetaCount(state.kind, cat.mcp.length);

  async function load() {
    setLoading(true);
    try {
      const next = await impexpApi.catalog();
      setCat(next);
      setLoaded(true);
      setLoadedAt(new Date().toISOString());
      setErr(null);
      setActionErr(catalogHasSecrets(next) ? t("impexp.leak") : "");
    } catch (e) {
      setErr(e);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  const selected = useMemo(() => selectionCount(sel), [sel]);

  async function runExport() {
    if (blocked) return;
    if (!selected) {
      setActionErr(t("impexp.needSelect"));
      return;
    }
    setBusy("export");
    try {
      const job = await impexpApi.export(sel);
      if (publicHasSecrets(job) || (job.archive && publicHasSecrets(job.archive))) {
        setActionErr(t("impexp.leak"));
        setExportJob(null);
      } else {
        setExportJob(job);
        setOk(t("impexp.exportOk"));
        setActionErr("");
        setTab("export");
      }
    } catch (e) {
      setActionErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  async function onFile(file: File | undefined) {
    if (blocked || !file) return;
    setBusy("preview");
    try {
      const text = await file.text();
      const parsed = parseArchiveFile(text);
      const prev = await impexpApi.preview(parsed);
      if (publicHasSecrets(prev)) {
        setActionErr(t("impexp.leak"));
        setPreview(null);
      } else {
        setPreview(prev);
        setImportJob(null);
        setActionErr(prev.valid ? "" : prev.errors.join("; ") || t("impexp.invalid"));
        setOk(prev.valid ? t("impexp.previewOk") : "");
      }
    } catch (e) {
      setActionErr(formatPublicError(e));
      setPreview(null);
    } finally {
      setBusy("");
    }
  }

  async function runImport(dry: boolean) {
    if (blocked) return;
    if (!preview?.archive) {
      setActionErr(t("impexp.needFile"));
      return;
    }
    if (!preview.valid) {
      setActionErr(t("impexp.invalid"));
      return;
    }
    if (!dry && !confirmImport) {
      setActionErr(t("impexp.needConfirm"));
      return;
    }
    setBusy(dry ? "dry" : "import");
    try {
      const job = await impexpApi.import(preview.archive, conflict, dry);
      if (publicHasSecrets(job)) {
        setActionErr(t("impexp.leak"));
        return;
      }
      setImportJob(job);
      if (job.report.failed.length) {
        setActionErr(t("impexp.report.failed"));
        setOk("");
      } else {
        setOk(dry ? t("impexp.dryOk") : t("impexp.importOk"));
        setActionErr("");
      }
      if (!dry) setConfirmImport(false);
      await load();
    } catch (e) {
      setActionErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  async function runRollback() {
    if (blocked || !importJob?.id || importJob.dry_run) return;
    setBusy("rollback");
    try {
      const job = await impexpApi.rollback(importJob.id);
      if (publicHasSecrets(job)) {
        setActionErr(t("impexp.leak"));
        return;
      }
      setImportJob(job);
      setOk(t("impexp.rollbackOk"));
      setActionErr("");
      await load();
    } catch (e) {
      setActionErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  return (
    <PageChrome
      icon="download"
      title={t("impexp.title")}
      description={t("impexp.desc")}
      primary={
        <Button variant="primary" disabled={blocked || !selected || Boolean(busy)} onClick={() => void runExport()}>
          {busy === "export" ? t("impexp.exporting") : t("impexp.export")}
        </Button>
      }
      refresh={
        <Button icon="refresh" iconGesture onClick={() => void load()} disabled={loading || Boolean(busy)}>
          {t("common.refresh")}
        </Button>
      }
    >
      <Card>
        <CardHeader icon="lock" title={t("impexp.how")} />
        <p style={{ margin: 0, padding: "0 16px 14px", fontSize: 12.5, color: "var(--text-3)", maxWidth: 720 }}>
          {t("impexp.howBody")}
        </p>
      </Card>
      <PageStatus kind={state.kind} errorText={err ? formatPublicError(err) : ""} staleAt={formatStaleAt(loadedAt, locale)} onReload={() => void load()} />
      {actionErr ? <StatusLine kind="error">{actionErr}</StatusLine> : null}
      {ok && !actionErr ? (
        <p role="status" style={{ margin: 0, fontSize: 12.5, color: "var(--green)" }}>
          {ok}
        </p>
      ) : null}

      <div style={{ display: "flex", gap: 6, flexWrap: "wrap" }}>
        {(Object.keys(TAB_KEYS) as TabId[]).map((id) => (
          <Button key={id} variant={tab === id ? "accent" : "quiet"} onClick={() => setTab(id)}>
            {t(TAB_KEYS[id])}
          </Button>
        ))}
      </div>

      {tab === "teams" ? (
        <Card>
          <CardHeader icon="layers" title={t("impexp.tab.teams")} meta={teamsMeta == null ? "—" : t("impexp.meta", { n: teamsMeta })} />
          {!blocked && loaded && shownCat.teams.length === 0 ? <EmptyState data-page-state="empty">{t("impexp.empty.teams")}</EmptyState> : null}
          {shownCat.teams.map((row) =>
            checkRow(row.id, row.name, sel.team_ids.includes(row.id), row.id, () => setSel((s) => ({ ...s, team_ids: toggleId(s.team_ids, row.id) })), blocked),
          )}
        </Card>
      ) : null}

      {tab === "agents" ? (
        <Card>
          <CardHeader icon="bolt" title={t("impexp.tab.agents")} meta={agentsMeta == null ? "—" : t("impexp.meta", { n: agentsMeta })} />
          {!blocked && loaded && shownCat.agents.length === 0 ? <EmptyState data-page-state="empty">{t("impexp.empty.agents")}</EmptyState> : null}
          {shownCat.agents.map((row) =>
            checkRow(
              row.id,
              row.display_name || row.agent_key,
              sel.agent_ids.includes(row.id),
              row.agent_key,
              () => setSel((s) => ({ ...s, agent_ids: toggleId(s.agent_ids, row.id) })),
              blocked,
            ),
          )}
        </Card>
      ) : null}

      {tab === "skills" ? (
        <>
          <Card>
            <CardHeader icon="build" title={t("impexp.skills")} meta={skillsMeta == null ? "—" : t("impexp.meta", { n: skillsMeta })} />
            {state.showItems && !cat.skills_configured ? <p style={{ margin: 0, padding: "0 16px 10px", fontSize: 12.5, color: "var(--text-3)" }}>{t("impexp.skillsOff")}</p> : null}
            {!blocked && loaded && shownCat.skills.length === 0 ? <EmptyState data-page-state="empty">{t("impexp.empty.skills")}</EmptyState> : null}
            {shownCat.skills.map((row) =>
              checkRow(row.name, row.name, sel.skill_names.includes(row.name), row.path || "", () =>
                setSel((s) => ({ ...s, skill_names: toggleId(s.skill_names, row.name) })), blocked,
              ),
            )}
          </Card>
          <Card>
            <CardHeader icon="hook" title={t("impexp.mcp")} meta={mcpMeta == null ? "—" : t("impexp.meta", { n: mcpMeta })} />
            {!blocked && loaded && shownCat.mcp.length === 0 ? <EmptyState data-page-state="empty">{t("impexp.empty.mcp")}</EmptyState> : null}
            {shownCat.mcp.map((row) =>
              checkRow(
                row.name,
                row.name,
                sel.mcp_names.includes(row.name),
                row.token_set ? t("impexp.tokenSet") : row.transport,
                () => setSel((s) => ({ ...s, mcp_names: toggleId(s.mcp_names, row.name) })),
                blocked,
              ),
            )}
          </Card>
        </>
      ) : null}

      {tab === "export" ? (
        <>
          <Card>
            <CardHeader icon="download" title={t("impexp.tab.export")} meta={t("impexp.selected", { n: selected })} />
            <div style={{ padding: "0 16px 16px", display: "flex", flexDirection: "column", gap: 10 }}>
              <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("impexp.exportHint")}</p>
              {!blocked && !selected && loaded ? <EmptyState>{t("impexp.empty.select")}</EmptyState> : null}
              <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
                <Button variant="accent" disabled={blocked || !selected || Boolean(busy)} onClick={() => void runExport()}>
                  {busy === "export" ? t("impexp.exporting") : t("impexp.export")}
                </Button>
                {exportJob?.archive ? (
                  <Button
                    icon="download"
                    disabled={blocked || Boolean(busy)}
                    onClick={() => downloadJSON(`goso-portable-${exportJob.id}.json`, exportJob.archive)}
                  >
                    {t("impexp.download")}
                  </Button>
                ) : null}
              </div>
            </div>
          </Card>
          {exportJob?.archive ? <ManifestCard manifest={exportJob.archive.manifest} title={t("impexp.manifest")} /> : null}
          {exportJob ? <ReportCard job={exportJob} t={t} /> : null}
        </>
      ) : null}

      {tab === "import" ? (
        <>
          <Card>
            <CardHeader icon="doc" title={t("impexp.tab.import")} />
            <div style={{ padding: "0 16px 16px", display: "flex", flexDirection: "column", gap: 10 }}>
              <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("impexp.importHint")}</p>
              <input
                type="file"
                accept="application/json,.json"
                aria-label={t("impexp.file")}
                disabled={blocked}
                onChange={(e) => void onFile(e.target.files?.[0])}
              />
              <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
                {CONFLICTS.map((c) => (
                  <label key={c} style={{ display: "flex", alignItems: "center", gap: 6, fontSize: 12.5 }}>
                    <input type="radio" name="conflict" checked={conflict === c} disabled={blocked} onChange={() => setConflict(c)} />
                    {t(CONFLICT_KEYS[c])}
                  </label>
                ))}
              </div>
              <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
                <Button disabled={blocked || !preview?.valid || Boolean(busy)} onClick={() => void runImport(true)}>
                  {busy === "dry" ? t("impexp.running") : t("impexp.dry")}
                </Button>
                <Button variant="accent" disabled={blocked || !preview?.valid || Boolean(busy) || !confirmImport} onClick={() => void runImport(false)}>
                  {busy === "import" ? t("impexp.importing") : t("impexp.import")}
                </Button>
                {importJob && !importJob.dry_run && importJob.kind === "import" && importJob.status === "done" ? (
                  <Button disabled={blocked || Boolean(busy)} onClick={() => void runRollback()}>
                    {t("impexp.rollback")}
                  </Button>
                ) : null}
              </div>
              <label style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 12.5 }}>
                <input type="checkbox" checked={confirmImport} disabled={blocked} onChange={(e) => setConfirmImport(e.target.checked)} />
                {t("impexp.confirm")}
              </label>
            </div>
          </Card>
          {preview ? <ManifestCard manifest={preview.manifest} title={t("impexp.manifest")} /> : null}
          {preview?.conflicts.length ? (
            <Card>
              <CardHeader icon="flag" title={t("impexp.conflicts")} meta={String(preview.conflicts.length)} />
              {preview.conflicts.map((c, i) => (
                <div key={`${c.kind}-${c.name}-${i}`} style={{ padding: "8px 16px", fontSize: 12.5, borderTop: "1px solid var(--border-soft)" }}>
                  {c.kind} · {c.name}
                </div>
              ))}
            </Card>
          ) : null}
          {preview && !preview.valid ? <EmptyState>{t("impexp.invalid")}</EmptyState> : null}
          {importJob ? <ReportCard job={importJob} t={t} /> : null}
        </>
      ) : null}
    </PageChrome>
  );
}
