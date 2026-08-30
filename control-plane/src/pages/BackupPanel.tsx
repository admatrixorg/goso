import { useEffect, useState } from "react";
import { backupApi, type BackupFile, type Preflight, type RestorePlan, type S3Status } from "../api/backup";
import {
  asPublicFile,
  confirmMatches,
  emptyPreflight,
  emptyS3,
  filterByScope,
  publicHasSecrets,
  type BackupDest,
  type BackupScope,
} from "../api/backup-ops";
import { useI18n, type MsgKey } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

type TabId = "system" | "tenant" | "restore" | "s3";

const TAB_KEYS: Record<TabId, MsgKey> = {
  system: "settings.backup.tab.system",
  tenant: "settings.backup.tab.tenant",
  restore: "settings.backup.tab.restore",
  s3: "settings.backup.tab.s3",
};

function triggerDownload(file: string, blob: Blob) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = file;
  a.click();
  URL.revokeObjectURL(url);
}

export function BackupPanel() {
  const { t } = useI18n();
  const [tab, setTab] = useState<TabId>("system");
  const [files, setFiles] = useState<BackupFile[]>([]);
  const [pf, setPf] = useState<Preflight>(emptyPreflight());
  const [s3, setS3] = useState<S3Status>(emptyS3());
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState("");
  const [ok, setOk] = useState("");
  const [busy, setBusy] = useState("");
  const [progress, setProgress] = useState(0);
  const [steps, setSteps] = useState<{ name: string; status: string; detail?: string }[]>([]);
  const [tenant, setTenant] = useState("default");
  const [dest, setDest] = useState<BackupDest>("local");
  const [selected, setSelected] = useState("");
  const [plan, setPlan] = useState<RestorePlan | null>(null);
  const [typed, setTyped] = useState("");
  const [confirm, setConfirm] = useState(false);
  const [endpoint, setEndpoint] = useState("");
  const [bucket, setBucket] = useState("");
  const [region, setRegion] = useState("");
  const [prefix, setPrefix] = useState("");
  const [accessKey, setAccessKey] = useState("");
  const [secret, setSecret] = useState("");

  async function load() {
    setLoading(true);
    try {
      const [list, pre, remote] = await Promise.all([backupApi.list(), backupApi.preflight(), backupApi.s3()]);
      if (list.files.some((f) => publicHasSecrets(f)) || publicHasSecrets(pre) || publicHasSecrets(remote)) {
        setErr(t("settings.backup.leak"));
        setFiles([]);
      } else {
        setFiles(list.files);
        setPf(pre);
        setS3(remote);
        setEndpoint(remote.endpoint || "");
        setBucket(remote.bucket || "");
        setRegion(remote.region || "");
        setPrefix(remote.prefix || "");
        setAccessKey("");
        setSecret("");
        setErr("");
      }
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  async function runCreate(scope: BackupScope) {
    if (!pf.can_backup) {
      setErr(t("settings.backup.blocked", { reason: pf.blocking || "" }));
      return;
    }
    setBusy("create");
    setProgress(15);
    setOk("");
    try {
      const snap = await backupApi.create(scope, scope === "tenant" ? tenant : "", dest);
      const pub = asPublicFile(snap);
      if (!pub) {
        setErr(t("settings.backup.leak"));
        return;
      }
      setProgress(pub.progress || 100);
      setSteps(pub.steps || []);
      setOk(pub.warning ? `${t("settings.backup.created", { file: pub.file })} — ${pub.warning}` : t("settings.backup.created", { file: pub.file }));
      setErr("");
      await load();
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setBusy("");
      setProgress(0);
    }
  }

  async function runDownload(file: string) {
    setBusy("download");
    try {
      const blob = await backupApi.download(file);
      triggerDownload(file, blob);
      setOk(t("settings.backup.downloaded", { file }));
      setErr("");
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  async function runPlan(file: string) {
    if (!file) {
      setErr(t("settings.backup.needFile"));
      return;
    }
    setBusy("plan");
    try {
      const next = await backupApi.plan(file);
      if (publicHasSecrets(next)) {
        setErr(t("settings.backup.leak"));
        setPlan(null);
        return;
      }
      setPlan(next);
      setSelected(file);
      setOk(t("settings.backup.planOk"));
      setErr("");
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  async function runRestore() {
    if (!selected) {
      setErr(t("settings.backup.needFile"));
      return;
    }
    if (!confirm || !confirmMatches(selected, typed)) {
      setErr(t("settings.backup.confirmMismatch"));
      return;
    }
    setBusy("restore");
    try {
      const r = await backupApi.restore(selected, typed);
      setOk(t("settings.backup.restored", { file: r.file || selected }));
      setErr("");
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  async function saveS3() {
    setBusy("s3");
    try {
      const row = await backupApi.putS3({ endpoint, bucket, region, prefix, access_key: accessKey, secret });
      if (publicHasSecrets(row)) {
        setErr(t("settings.backup.leak"));
        return;
      }
      setS3(row);
      setAccessKey("");
      setSecret("");
      setOk(t("settings.backup.s3.saved"));
      setErr("");
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  const systemFiles = filterByScope(files, "system");
  const tenantFiles = filterByScope(files, "tenant");
  const listForTab = tab === "tenant" ? tenantFiles : systemFiles;

  function fileTable(rows: BackupFile[], emptyKey: MsgKey) {
    return (
      <Card>
        <CardHeader icon="download" title={t("settings.backup")} meta={String(rows.length)} />
        <TableScroll>
          <div style={{ display: "flex", gap: 12, padding: "8px 16px", fontSize: 11, fontWeight: 700, color: "var(--text-3)" }}>
            <span style={{ flex: 2 }}>{t("settings.col.file")}</span>
            <span style={{ flex: 0.8 }}>{t("settings.col.bytes")}</span>
            <span style={{ flex: 1 }}>{t("settings.backup.scope")}</span>
            <span style={{ flex: 0.8 }}>{t("settings.col.integrity")}</span>
            <span style={{ width: 210 }} />
          </div>
          {rows.map((b) => (
            <div key={b.file} style={{ display: "flex", gap: 12, padding: "8px 16px", borderTop: "1px solid var(--border-soft)", fontSize: 13, alignItems: "center" }}>
              <span style={{ flex: 2, fontFamily: "var(--font-mono, ui-monospace)", fontSize: 12, fontWeight: 600 }}>{b.file}</span>
              <span style={{ flex: 0.8, color: "var(--text-2)", fontVariantNumeric: "tabular-nums" }}>{b.bytes}</span>
              <span style={{ flex: 1, color: "var(--text-3)" }}>{b.scope || "system"}{b.tenant ? ` · ${b.tenant}` : ""}</span>
              <span style={{ flex: 0.8 }}>
                <Badge tone={b.integrity === "ok" ? "positive" : "critical"}>{b.integrity === "ok" ? t("settings.backup.ok") : t("settings.backup.fail")}</Badge>
              </span>
              <span style={{ width: 210, display: "flex", gap: 6, justifyContent: "flex-end", flexWrap: "wrap" }}>
                <Button variant="quiet" style={{ padding: "4px 8px" }} disabled={Boolean(busy)} onClick={() => void runDownload(b.file)}>
                  {t("settings.backup.download")}
                </Button>
                <Button variant="quiet" style={{ padding: "4px 8px" }} disabled={Boolean(busy)} onClick={() => void runPlan(b.file)}>
                  {t("settings.backup.plan")}
                </Button>
              </span>
            </div>
          ))}
          {!loading && rows.length === 0 ? <EmptyState>{t(emptyKey)}</EmptyState> : null}
        </TableScroll>
      </Card>
    );
  }

  return (
    <>
      <div style={{ fontSize: 21, fontWeight: 700 }}>{t("settings.backup")}</div>
      <div style={{ fontSize: 12.5, color: "var(--text-3)" }}>{t("settings.backup.desc")}</div>
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
        {(Object.keys(TAB_KEYS) as TabId[]).map((id) => (
          <Button key={id} variant={tab === id ? "primary" : "quiet"} onClick={() => setTab(id)}>
            {t(TAB_KEYS[id])}
          </Button>
        ))}
        <Button icon="refresh" iconGesture disabled={loading || Boolean(busy)} onClick={() => void load()}>
          {t("common.refresh")}
        </Button>
      </div>
      {loading ? <StatusLine kind="loading" /> : null}
      {err ? <StatusLine kind="error">{err}</StatusLine> : null}
      {ok && !err ? <p role="status" style={{ margin: 0, fontSize: 12.5, color: "var(--green)" }}>{ok}</p> : null}
      {busy && progress > 0 ? (
        <div role="progressbar" aria-valuenow={progress} aria-valuemin={0} aria-valuemax={100} style={{ height: 6, borderRadius: 99, background: "var(--border-soft)", overflow: "hidden" }}>
          <div style={{ width: `${progress}%`, height: "100%", background: "var(--accent)" }} />
        </div>
      ) : null}
      {steps.length ? (
        <div style={{ fontSize: 12.5, color: "var(--text-2)", display: "flex", flexDirection: "column", gap: 4 }}>
          {steps.map((s, i) => (
            <div key={`${s.name}-${i}`}>
              {s.name}: {s.status}
              {s.detail ? ` · ${s.detail}` : ""}
            </div>
          ))}
        </div>
      ) : null}

      {tab === "system" || tab === "tenant" ? (
        <>
          <Card>
            <CardHeader icon="pulse" title={t("settings.backup.preflight")} meta={pf.engine || "—"} />
            <div style={{ padding: "0 16px 14px", display: "flex", flexDirection: "column", gap: 8 }}>
              {pf.checks.map((c) => (
                <div key={c.id} style={{ display: "flex", gap: 8, alignItems: "center", fontSize: 12.5 }}>
                  <Badge tone={c.ok ? "positive" : c.blocking ? "critical" : "warning"}>{c.ok ? t("settings.backup.check.ok") : t("settings.backup.check.fail")}</Badge>
                  <span style={{ fontWeight: 600 }}>{c.id}</span>
                  <span style={{ color: "var(--text-3)" }}>{c.detail}</span>
                </div>
              ))}
              {!loading && pf.checks.length === 0 ? <EmptyState>{t("settings.backup.preflight.empty")}</EmptyState> : null}
              {!pf.can_backup ? <p style={{ margin: 0, color: "var(--red)", fontSize: 12.5 }}>{t("settings.backup.blocked", { reason: pf.blocking || "" })}</p> : null}
            </div>
          </Card>
          <div style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "center" }}>
            {tab === "tenant" ? (
              <input className="z-field" aria-label={t("settings.backup.tenant")} value={tenant} onChange={(e) => setTenant(e.target.value)} style={{ minWidth: 160 }} />
            ) : null}
            <label style={{ display: "flex", gap: 6, alignItems: "center", fontSize: 12.5, color: "var(--text-2)" }}>
              <input type="checkbox" checked={dest === "s3"} onChange={(e) => setDest(e.target.checked ? "s3" : "local")} disabled={!s3.configured} />
              {t("settings.backup.dest.s3")}
            </label>
            <Button
              variant="primary"
              icon="plus"
              disabled={Boolean(busy) || !pf.can_backup}
              onClick={() => void runCreate(tab === "tenant" ? "tenant" : "system")}
            >
              {tab === "tenant" ? t("settings.backup.createTenant") : t("settings.backup.create")}
            </Button>
          </div>
          {fileTable(listForTab, tab === "tenant" ? "settings.backup.emptyTenant" : "settings.backup.empty")}
        </>
      ) : null}

      {tab === "restore" ? (
        <>
          <Card>
            <CardHeader icon="history" title={t("settings.backup.tab.restore")} />
            <div style={{ padding: "0 16px 16px", display: "flex", flexDirection: "column", gap: 10 }}>
              <select className="z-field" aria-label={t("settings.col.file")} value={selected} onChange={(e) => setSelected(e.target.value)}>
                <option value="">{t("settings.backup.needFile")}</option>
                {files.map((f) => (
                  <option key={f.file} value={f.file}>
                    {f.file}
                  </option>
                ))}
              </select>
              {!loading && files.length === 0 ? <EmptyState>{t("settings.backup.empty")}</EmptyState> : null}
              <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
                <Button disabled={!selected || Boolean(busy)} onClick={() => void runPlan(selected)}>
                  {t("settings.backup.plan")}
                </Button>
                <Button disabled={!selected || Boolean(busy)} onClick={() => void backupApi.validate(selected).then(() => setOk(t("settings.backup.validated"))).catch((e) => setErr(formatPublicError(e)))}>
                  {t("settings.backup.validate")}
                </Button>
              </div>
              <label style={{ display: "flex", gap: 8, alignItems: "center", fontSize: 12.5 }}>
                <input type="checkbox" checked={confirm} onChange={(e) => setConfirm(e.target.checked)} />
                {t("settings.backup.confirmApply")}
              </label>
              <input
                className="z-field"
                aria-label={t("settings.backup.confirmLabel")}
                placeholder={selected || t("settings.backup.confirmLabel")}
                value={typed}
                onChange={(e) => setTyped(e.target.value)}
              />
              <Button variant="primary" disabled={!confirm || !confirmMatches(selected, typed) || Boolean(busy)} onClick={() => void runRestore()}>
                {t("settings.backup.restore")}
              </Button>
              <p style={{ margin: 0, fontSize: 12, color: "var(--text-3)", lineHeight: 1.5 }}>{t("settings.backup.applyHint")}</p>
            </div>
          </Card>
          {plan ? (
            <Card>
              <CardHeader icon="list" title={t("settings.backup.plan")} meta={plan.valid ? t("settings.backup.ok") : t("settings.backup.fail")} />
              <div style={{ padding: "0 16px 16px", display: "flex", flexDirection: "column", gap: 6, fontSize: 12.5 }}>
                <div>{t("settings.backup.credsExcluded")}: {String(plan.credentials_excluded)}</div>
                <div>{t("settings.backup.recovery")}: {plan.recovery.strategy}</div>
                {plan.actions.map((a) => (
                  <div key={a}>{a}</div>
                ))}
                {plan.errors.map((e) => (
                  <div key={e} style={{ color: "var(--red)" }}>{e}</div>
                ))}
                {plan.warnings.map((w) => (
                  <div key={w} style={{ color: "var(--text-3)" }}>{w}</div>
                ))}
              </div>
            </Card>
          ) : null}
        </>
      ) : null}

      {tab === "s3" ? (
        <Card>
          <CardHeader icon="cloud" title={t("settings.backup.tab.s3")} meta={s3.configured ? t("settings.backup.s3.configured") : t("settings.backup.s3.missing")} />
          <div style={{ padding: "0 16px 16px", display: "flex", flexDirection: "column", gap: 10 }}>
            <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("settings.backup.s3.desc")}</p>
            <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("settings.backup.s3.hint")}</p>
            {s3.env_owned ? <p style={{ margin: 0, fontSize: 12.5, color: "var(--orange)" }}>{t("settings.backup.s3.env")}</p> : null}
            <input className="z-field" placeholder={t("settings.backup.s3.endpoint")} value={endpoint} onChange={(e) => setEndpoint(e.target.value)} disabled={Boolean(s3.env_owned)} />
            <input className="z-field" placeholder={t("settings.backup.s3.bucket")} value={bucket} onChange={(e) => setBucket(e.target.value)} disabled={Boolean(s3.env_owned)} />
            <input className="z-field" placeholder={t("settings.backup.s3.region")} value={region} onChange={(e) => setRegion(e.target.value)} disabled={Boolean(s3.env_owned)} />
            <input className="z-field" placeholder={t("settings.backup.s3.prefix")} value={prefix} onChange={(e) => setPrefix(e.target.value)} disabled={Boolean(s3.env_owned)} />
            <input className="z-field" type="password" autoComplete="off" placeholder={t("settings.backup.s3.access")} value={accessKey} onChange={(e) => setAccessKey(e.target.value)} disabled={Boolean(s3.env_owned)} />
            <input className="z-field" type="password" autoComplete="off" placeholder={t("settings.backup.s3.secret")} value={secret} onChange={(e) => setSecret(e.target.value)} disabled={Boolean(s3.env_owned)} />
            <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
              <Button variant="primary" disabled={Boolean(busy) || Boolean(s3.env_owned)} onClick={() => void saveS3()}>
                {t("settings.backup.s3.save")}
              </Button>
              <Button disabled={Boolean(busy) || !s3.configured} onClick={() => void backupApi.testS3().then(() => setOk(t("settings.backup.s3.testOk"))).catch((e) => setErr(formatPublicError(e)))}>
                {t("settings.backup.s3.test")}
              </Button>
              <Button
                disabled={Boolean(busy) || Boolean(s3.env_owned)}
                onClick={() =>
                  void backupApi
                    .clearS3(bucket || "s3")
                    .then((row) => {
                      if (publicHasSecrets(row)) setErr(t("settings.backup.leak"));
                      else {
                        setS3(row);
                        setOk(t("settings.backup.s3.cleared"));
                      }
                    })
                    .catch((e) => setErr(formatPublicError(e)))
                }
              >
                {t("settings.backup.s3.clear")}
              </Button>
            </div>
          </div>
        </Card>
      ) : null}
    </>
  );
}
