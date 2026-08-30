import { useEffect, useMemo, useState } from "react";
import { packagesApi, type AllowEntry, type Pkg } from "../api/packages";
import {
  CLI_KINDS,
  ECOSYSTEMS,
  allowConfirmMatch,
  asPublicSnapshot,
  cliConfirmMatch,
  filterByEco,
  jobActive,
  latestJob,
  pinValid,
  pkgConfirmMatch,
  pkgLabel,
  publicHasSecrets,
  runtimeForEco,
  snapshotHasSecrets,
  type CLIKind,
  type Ecosystem,
  type PkgJob,
  type Snapshot,
} from "../api/packages-ops";
import { classifyPageState, formatStaleAt, inventoryBlocksMutation, listMetaCount } from "../api/page-state";
import { useI18n, type MsgKey } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { PageChrome } from "../ui/PageChrome";
import { PageStatus } from "../ui/PageStatus";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

type TabId = Ecosystem | "cli";
type ConfirmKind = "uninstall" | "recover" | "unpin" | "uncli";

const emptySnap = (): Snapshot => asPublicSnapshot({});

const TAB_KEYS: Record<TabId, MsgKey> = {
  system: "pkg.tab.system",
  python: "pkg.tab.python",
  node: "pkg.tab.node",
  github: "pkg.tab.github",
  cli: "pkg.tab.cli",
};

const KIND_KEYS: Record<CLIKind, MsgKey> = {
  github: "pkg.cli.kind.github",
  npm: "pkg.cli.kind.npm",
  pypi: "pkg.cli.kind.pypi",
};

function statusTone(st: string): "positive" | "warning" | "critical" | "neutral" | "accent" {
  if (st === "installed") return "positive";
  if (st === "partial" || st === "installing" || st === "uninstalling") return "warning";
  if (st === "failed") return "critical";
  return "neutral";
}

function statusKey(st: string): MsgKey {
  if (st === "installed") return "pkg.status.installed";
  if (st === "installing") return "pkg.status.installing";
  if (st === "partial") return "pkg.status.partial";
  if (st === "failed") return "pkg.status.failed";
  if (st === "uninstalling") return "pkg.status.uninstalling";
  return "pkg.status.missing";
}

function jobOkKey(job: PkgJob): MsgKey {
  if (job.status === "partial") return "pkg.partialOk";
  if (job.status === "failed") return "pkg.failOk";
  if (job.action === "uninstall") return "pkg.uninstallOk";
  if (job.action === "recover") return "pkg.recoverOk";
  return "pkg.installOk";
}

export function PackagesPage() {
  const { t, locale } = useI18n();
  const [snap, setSnap] = useState<Snapshot>(emptySnap);
  const [tab, setTab] = useState<TabId>("python");
  const [q, setQ] = useState("");
  const [loading, setLoading] = useState(true);
  const [loaded, setLoaded] = useState(false);
  const [loadedAt, setLoadedAt] = useState<string | null>(null);
  const [err, setErr] = useState<unknown>(null);
  const [actionErr, setActionErr] = useState("");
  const [ok, setOk] = useState("");
  const [busy, setBusy] = useState("");
  const [name, setName] = useState("");
  const [pin, setPin] = useState("");
  const [confirmInstall, setConfirmInstall] = useState("");
  const [cliDraft, setCliDraft] = useState<Record<string, string>>({});
  const [confirm, setConfirm] = useState<{ kind: ConfirmKind; pkg?: Pkg; allow?: AllowEntry; cli?: CLIKind } | null>(null);
  const [typed, setTyped] = useState("");
  const na = t("pkg.na");

  const inventoryCount = snap.runtimes.length + snap.packages.length + snap.allowlist.length;
  const state = classifyPageState({
    loading,
    loaded,
    error: err,
    itemCount: inventoryCount,
    keepStale: loaded && inventoryCount > 0,
  });
  const blocked = inventoryBlocksMutation(state.kind);
  const metaN = listMetaCount(state.kind, snap.packages.length);

  async function load(quiet = false) {
    if (!quiet) setLoading(true);
    try {
      const next = await packagesApi.snapshot();
      setSnap(next);
      setLoaded(true);
      setLoadedAt(new Date().toISOString());
      setErr(null);
      setActionErr(snapshotHasSecrets(next) ? t("pkg.leak") : "");
    } catch (e) {
      setErr(e);
    } finally {
      if (!quiet) setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  const active = snap.jobs.some(jobActive);
  useEffect(() => {
    if (!active) return;
    const id = window.setInterval(() => void load(true), 1500);
    return () => window.clearInterval(id);
  }, [active]);

  const eco: Ecosystem = tab === "cli" ? "python" : tab;
  const allows = useMemo(() => filterByEco(state.showItems ? snap.allowlist : [], eco), [snap.allowlist, eco, state.showItems]);
  const pkgs = useMemo(() => filterByEco(state.showItems ? snap.packages : [], eco, q), [snap.packages, eco, q, state.showItems]);
  const rt = runtimeForEco(state.showItems ? snap.runtimes : [], eco);
  const job = latestJob(state.showItems ? snap.jobs : [], undefined, tab === "cli" ? undefined : eco);
  const runtimeReady = Boolean(rt?.present);
  const pkgEmpty = !blocked && tab !== "cli" && loaded && !err && pkgs.length === 0;
  const pkgFilterEmpty = pkgEmpty && q.trim().length > 0;
  const pkgTrueEmpty = pkgEmpty && !q.trim();

  const matched = confirm
    ? confirm.kind === "unpin" && confirm.allow
      ? allowConfirmMatch(typed, confirm.allow)
      : confirm.kind === "uncli" && confirm.cli
        ? cliConfirmMatch(typed, confirm.cli)
        : confirm.pkg
          ? pkgConfirmMatch(typed, confirm.pkg)
          : false
    : false;

  function afterJob(job: PkgJob) {
    setOk(t(jobOkKey(job)));
    setActionErr("");
  }

  async function addAllow() {
    if (blocked) return;
    if (!pinValid(pin)) {
      setActionErr(t("pkg.pinBad"));
      return;
    }
    setBusy("allow");
    try {
      await packagesApi.allow({ ecosystem: eco, name: name.trim(), pin: pin.trim() });
      setOk(t("pkg.allowOk"));
      setActionErr("");
      setName("");
      setPin("");
      await load(true);
    } catch (e) {
      setActionErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  async function install() {
    if (blocked || !runtimeReady) return;
    const version = pin.trim();
    if (!pinValid(version)) {
      setActionErr(t("pkg.pinBad"));
      return;
    }
    if (!confirmInstall.trim()) {
      setActionErr(t("pkg.mismatch"));
      return;
    }
    setBusy("install");
    try {
      const res = await packagesApi.install({
        ecosystem: eco,
        name: name.trim(),
        version,
        confirm: confirmInstall.trim(),
      });
      if (publicHasSecrets(res.package) || publicHasSecrets(res.job)) {
        setActionErr(t("pkg.leak"));
      } else {
        afterJob(res.job);
      }
      setConfirmInstall("");
      await load(true);
    } catch (e) {
      setActionErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  async function saveCLI(kind: CLIKind) {
    if (blocked) return;
    const token = (cliDraft[kind] || "").trim();
    if (!token) return;
    setBusy("cli-" + kind);
    try {
      const row = await packagesApi.setCLI(kind, token);
      if (publicHasSecrets(row)) setActionErr(t("pkg.leak"));
      else {
        setOk(t("pkg.cliOk"));
        setActionErr("");
      }
      setCliDraft((d) => ({ ...d, [kind]: "" }));
      await load(true);
    } catch (e) {
      setActionErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  async function runConfirm() {
    if (blocked || !confirm || !matched) {
      setActionErr(t("pkg.mismatch"));
      return;
    }
    setBusy("confirm");
    try {
      if (confirm.kind === "uninstall" && confirm.pkg) {
        const res = await packagesApi.uninstall(confirm.pkg.id, typed.trim());
        afterJob(res.job);
      } else if (confirm.kind === "recover" && confirm.pkg) {
        const res = await packagesApi.recover(confirm.pkg.id, typed.trim());
        afterJob(res.job);
      } else if (confirm.kind === "unpin" && confirm.allow) {
        await packagesApi.unpin(confirm.allow.id, typed.trim());
        setOk(t("pkg.unpinOk"));
        setActionErr("");
      } else if (confirm.kind === "uncli" && confirm.cli) {
        const row = await packagesApi.clearCLI(confirm.cli, typed.trim());
        if (publicHasSecrets(row)) setActionErr(t("pkg.leak"));
        else {
          setOk(t("pkg.cliClearOk"));
          setActionErr("");
        }
      }
      setConfirm(null);
      setTyped("");
      await load(true);
    } catch (e) {
      setActionErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  function confirmPreview(): string {
    if (!confirm) return "";
    if (confirm.kind === "uninstall" && confirm.pkg) return t("pkg.confirmUninstall", { name: pkgLabel(confirm.pkg) });
    if (confirm.kind === "recover" && confirm.pkg) return t("pkg.confirmRecover", { name: pkgLabel(confirm.pkg) });
    if (confirm.kind === "unpin" && confirm.allow) return t("pkg.confirmUnpin", { name: confirm.allow.name });
    if (confirm.kind === "uncli" && confirm.cli) return t("pkg.confirmUncli", { name: t(KIND_KEYS[confirm.cli]) });
    return "";
  }

  return (
    <PageChrome
      icon="build"
      title={t("pkg.title")}
      description={t("pkg.desc")}
      primary={
        <Button icon="refresh" iconGesture variant="primary" onClick={() => void load()} disabled={loading || Boolean(busy)}>
          {t("common.refresh")}
        </Button>
      }
      filters={
        tab !== "cli" ? (
          <input
            className="z-field"
            value={q}
            disabled={blocked}
            onChange={(e) => setQ(e.target.value)}
            placeholder={t("pkg.search")}
            aria-label={t("pkg.search")}
            autoComplete="off"
            spellCheck={false}
            style={{ minWidth: 220, flex: 1 }}
          />
        ) : undefined
      }
    >
      <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("pkg.hint")}</p>
      <PageStatus kind={state.kind} errorText={err ? formatPublicError(err) : ""} staleAt={formatStaleAt(loadedAt, locale)} onReload={() => void load()} />
      {actionErr ? <StatusLine kind="error">{actionErr}</StatusLine> : null}
      {ok && !actionErr ? (
        <p role="status" style={{ margin: 0, fontSize: 12.5, color: "var(--green)" }}>
          {ok}
        </p>
      ) : null}

      <Card>
        <CardHeader icon="pulse" title={t("pkg.runtime")} />
        {state.showEmpty ? <EmptyState data-page-state="empty">{t("pkg.runtime.empty")}</EmptyState> : null}
        <div style={{ display: "flex", gap: 8, flexWrap: "wrap", padding: "0 16px 16px" }}>
          {(state.showItems ? snap.runtimes : []).map((r) => (
            <div key={r.name} style={{ minWidth: 140, padding: "8px 10px", border: "1px solid var(--border)", borderRadius: 8 }}>
              <div style={{ fontWeight: 600, fontSize: 13 }}>{r.name}</div>
              <div style={{ fontSize: 12, color: "var(--text-3)" }}>{r.version || na}</div>
              <div style={{ marginTop: 6, display: "flex", gap: 6, flexWrap: "wrap" }}>
                <Badge tone={r.present ? "positive" : "critical"}>{r.present ? t("pkg.present") : t("pkg.missing")}</Badge>
                <Badge tone={r.compatible ? "accent" : "warning"}>{r.compatible ? t("pkg.compatible") : t("pkg.incompatible")}</Badge>
              </div>
              {r.warning ? <p style={{ margin: "6px 0 0", fontSize: 12, color: "var(--orange)" }}>{r.warning}</p> : null}
            </div>
          ))}
        </div>
      </Card>

      <div style={{ display: "flex", gap: 6, flexWrap: "wrap" }} role="tablist" aria-label={t("pkg.title")}>
        {([...ECOSYSTEMS, "cli"] as TabId[]).map((id) => (
          <Button key={id} variant={tab === id ? "accent" : "quiet"} onClick={() => setTab(id)}>
            {t(TAB_KEYS[id])}
          </Button>
        ))}
      </div>

      {tab !== "cli" ? (
        <>
          {rt && !rt.compatible ? (
            <p role="status" style={{ margin: 0, fontSize: 12.5, color: "var(--orange)" }}>
              {t("pkg.warn")}: {rt.warning || t("pkg.incompatible")}
            </p>
          ) : null}

          <Card>
            <CardHeader icon="plus" title={t("pkg.allow")} />
            <div style={{ padding: "0 16px 16px", display: "flex", flexDirection: "column", gap: 10 }}>
              <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("pkg.allowHint")}</p>
              <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
                <input
                  className="z-field"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder={t("pkg.name")}
                  aria-label={t("pkg.name")}
                  autoComplete="off"
                  spellCheck={false}
                  disabled={blocked}
                  style={{ minWidth: 160, flex: 1 }}
                />
                <input
                  className="z-field"
                  value={pin}
                  disabled={blocked}
                  onChange={(e) => setPin(e.target.value)}
                  placeholder={t("pkg.pin")}
                  aria-label={t("pkg.pin")}
                  autoComplete="off"
                  spellCheck={false}
                  style={{ minWidth: 120 }}
                />
                <Button variant="accent" disabled={blocked || Boolean(busy) || !name.trim() || !pin.trim()} onClick={() => void addAllow()}>
                  {t("pkg.addAllow")}
                </Button>
                <Button
                  disabled={blocked || !runtimeReady || Boolean(busy) || !name.trim() || !pin.trim() || !confirmInstall.trim()}
                  onClick={() => void install()}
                >
                  {t("pkg.install")}
                </Button>
              </div>
              <input
                className="z-field"
                value={confirmInstall}
                onChange={(e) => setConfirmInstall(e.target.value)}
                placeholder={t("pkg.installHint")}
                aria-label={t("pkg.confirm")}
                autoComplete="off"
                spellCheck={false}
                disabled={blocked}
              />
              {!runtimeReady && !blocked ? (
                <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("pkg.runtimeMissing")}</p>
              ) : null}
              {!blocked && allows.length === 0 ? <EmptyState>{t("pkg.allowEmpty")}</EmptyState> : null}
              {allows.map((a) => (
                <div key={a.id} style={{ display: "flex", gap: 8, alignItems: "center", fontSize: 12.5 }}>
                  <code style={{ flex: 1 }}>
                    {a.name}@{a.pin}
                  </code>
                  <Button
                    variant="quiet"
                    disabled={blocked}
                    onClick={() => {
                      setConfirm({ kind: "unpin", allow: a });
                      setTyped("");
                    }}
                  >
                    {t("pkg.unpin")}
                  </Button>
                </div>
              ))}
            </div>
          </Card>

          <Card>
            <CardHeader icon="list" title={t("pkg.list")} meta={metaN == null ? "—" : t("pkg.meta", { n: pkgs.length })} />
            <TableScroll>
              <div className="z-row z-row-head" style={{ display: "flex", gap: 8, padding: "8px 16px", fontSize: 11, fontWeight: 700, color: "var(--text-3)" }}>
                <span style={{ flex: 1.4 }}>{t("pkg.col.name")}</span>
                <span style={{ flex: 0.8 }}>{t("pkg.col.version")}</span>
                <span style={{ flex: 0.9 }}>{t("pkg.col.status")}</span>
                <span style={{ flex: 1.4 }}>{t("pkg.col.warning")}</span>
                <span style={{ width: 140 }}>{t("pkg.col.actions")}</span>
              </div>
              {pkgTrueEmpty ? <EmptyState data-page-state="empty">{t("pkg.empty")}</EmptyState> : null}
              {pkgFilterEmpty ? <EmptyState data-page-state="filtered_empty">{t("pkg.filterEmpty")}</EmptyState> : null}
              {pkgs.map((row) => (
                <div key={row.id} className="z-row" style={{ display: "flex", gap: 8, padding: "10px 16px", alignItems: "center", fontSize: 13, borderTop: "1px solid var(--border)" }}>
                  <span style={{ flex: 1.4 }}>{row.name}</span>
                  <code style={{ flex: 0.8, fontSize: 12 }}>{row.version}</code>
                  <span style={{ flex: 0.9 }}>
                    <Badge tone={statusTone(row.status)}>{t(statusKey(row.status))}</Badge>
                  </span>
                  <span style={{ flex: 1.4, color: "var(--text-3)", fontSize: 12 }}>{row.warning || na}</span>
                  <span style={{ width: 140, display: "flex", gap: 4 }}>
                    {row.status === "partial" || row.status === "failed" ? (
                      <Button
                        variant="quiet"
                        disabled={blocked}
                        onClick={() => {
                          setConfirm({ kind: "recover", pkg: row });
                          setTyped("");
                        }}
                      >
                        {t("pkg.recover")}
                      </Button>
                    ) : (
                      <Button
                        variant="quiet"
                        disabled={blocked}
                        onClick={() => {
                          setConfirm({ kind: "uninstall", pkg: row });
                          setTyped("");
                        }}
                      >
                        {t("pkg.uninstall")}
                      </Button>
                    )}
                  </span>
                </div>
              ))}
            </TableScroll>
          </Card>

          <Card>
            <CardHeader icon="history" title={t("pkg.progress")} meta={job ? `${job.progress}%` : undefined} />
            <div style={{ padding: "0 16px 16px" }}>
              {!blocked && !job ? <EmptyState>{t("pkg.progressEmpty")}</EmptyState> : null}
              {job ? (
                <>
                  <div style={{ height: 8, background: "var(--surface-2)", borderRadius: 99, overflow: "hidden" }}>
                    <div style={{ width: `${job.progress}%`, height: "100%", background: job.status === "failed" ? "var(--red)" : "var(--accent)" }} />
                  </div>
                  <p style={{ margin: "8px 0 0", fontSize: 12.5, color: "var(--text-3)" }}>
                    {job.action} · {job.name} · {job.status}
                  </p>
                  <p style={{ margin: "12px 0 6px", fontSize: 12, fontWeight: 600, color: "var(--text-3)" }}>{t("pkg.logs")}</p>
                  {job.log.length === 0 ? <EmptyState>{t("pkg.logsEmpty")}</EmptyState> : null}
                  <pre style={{ margin: 0, fontSize: 12, whiteSpace: "pre-wrap", color: "var(--text-2)" }}>{job.log.join("\n")}</pre>
                  {job.error ? <p style={{ margin: "8px 0 0", color: "var(--red)", fontSize: 12.5 }}>{job.error}</p> : null}
                </>
              ) : null}
            </div>
          </Card>
        </>
      ) : (
        <Card>
          <CardHeader icon="lock" title={t("pkg.cli.title")} />
          <div style={{ padding: "0 16px 16px", display: "flex", flexDirection: "column", gap: 14 }}>
            <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("pkg.cli.desc")}</p>
            {snap.credentials.map((c) => {
              const kind = c.kind as CLIKind;
              return (
                <div key={c.kind} style={{ display: "flex", flexDirection: "column", gap: 8, paddingBottom: 8, borderBottom: "1px solid var(--border)" }}>
                  <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
                    <strong style={{ fontSize: 13 }}>{t(KIND_KEYS[kind] || "pkg.cli.kind.github")}</strong>
                    <Badge tone={c.set ? "positive" : "neutral"}>{c.set ? t("pkg.cli.set") : t("pkg.cli.unset")}</Badge>
                  </div>
                  <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
                    <input
                      className="z-field"
                      type="password"
                      value={cliDraft[c.kind] || ""}
                      disabled={blocked}
                      onChange={(e) => setCliDraft((d) => ({ ...d, [c.kind]: e.target.value }))}
                      placeholder={t("pkg.cli.token")}
                      aria-label={`${c.kind} ${t("pkg.cli.token")}`}
                      autoComplete="new-password"
                      spellCheck={false}
                      style={{ minWidth: 220, flex: 1 }}
                    />
                    <Button variant="accent" disabled={blocked || Boolean(busy) || !(cliDraft[c.kind] || "").trim()} onClick={() => void saveCLI(kind)}>
                      {t("pkg.cli.save")}
                    </Button>
                    <Button
                      variant="quiet"
                      disabled={blocked || Boolean(busy) || !c.set}
                      onClick={() => {
                        setConfirm({ kind: "uncli", cli: kind });
                        setTyped("");
                      }}
                    >
                      {t("pkg.cli.clear")}
                    </Button>
                  </div>
                </div>
              );
            })}
          </div>
        </Card>
      )}

      {confirm && !blocked ? (
        <Card>
          <CardHeader icon="shield" title={t("pkg.confirmTitle")} />
          <div style={{ padding: "0 16px 16px", display: "flex", flexDirection: "column", gap: 10 }}>
            <p style={{ margin: 0, fontSize: 13 }}>{confirmPreview()}</p>
            <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("pkg.confirmHint")}</p>
            <input
              className="z-field"
              value={typed}
              onChange={(e) => setTyped(e.target.value)}
              placeholder={t("pkg.confirmPlaceholder")}
              aria-label={t("pkg.confirmPlaceholder")}
              autoComplete="off"
              spellCheck={false}
            />
            <div style={{ display: "flex", gap: 8 }}>
              <Button variant="accent" disabled={!matched || Boolean(busy)} onClick={() => void runConfirm()}>
                {t("pkg.confirmGo")}
              </Button>
              <Button
                variant="quiet"
                onClick={() => {
                  setConfirm(null);
                  setTyped("");
                }}
              >
                {t("pkg.cancel")}
              </Button>
            </div>
          </div>
        </Card>
      ) : null}
    </PageChrome>
  );
}
