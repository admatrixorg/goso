import { useEffect, useState } from "react";
import { classifyPageState, formatStaleAt, inventoryBlocksMutation, listMetaCount } from "../api/page-state";
import { pendingApi, type PendingGroup } from "../api/pending";
import {
  agentLabel,
  asPublic,
  compactBlocked,
  formatAge,
  groupLabel,
  pendingConfirmMatch,
  pendingWriteKind,
  previewLine,
  publicHasSecrets,
} from "../api/pending-ops";
import { useI18n } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { PageChrome } from "../ui/PageChrome";
import { PageStatus } from "../ui/PageStatus";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

type ConfirmKind = "compact" | "clear";

export function PendingPage() {
  const { t, locale } = useI18n();
  const [groups, setGroups] = useState<PendingGroup[]>([]);
  const [loading, setLoading] = useState(true);
  const [loaded, setLoaded] = useState(false);
  const [loadedAt, setLoadedAt] = useState<string | null>(null);
  const [err, setErr] = useState<unknown>(null);
  const [actionErr, setActionErr] = useState("");
  const [ok, setOk] = useState("");
  const [busy, setBusy] = useState("");
  const [confirm, setConfirm] = useState<{ kind: ConfirmKind; group: PendingGroup } | null>(null);
  const [typed, setTyped] = useState("");
  const na = t("pending.na");
  const state = classifyPageState({
    loading,
    loaded,
    error: err,
    itemCount: groups.length,
    keepStale: loaded && groups.length > 0,
  });
  const blocked = inventoryBlocksMutation(state.kind);
  const metaN = listMetaCount(state.kind, groups.length);
  const inProgress = groups.some((g) => g.compacting) || busy.startsWith("compact:");
  const matched = confirm ? pendingConfirmMatch(typed, confirm.group) : false;

  async function load() {
    setLoading(true);
    try {
      const j = await pendingApi.list();
      const rows = asPublic(j.groups);
      setGroups(rows);
      setLoaded(true);
      setLoadedAt(new Date().toISOString());
      if (rows.some((g) => publicHasSecrets(g)) || (j.groups || []).some((g) => publicHasSecrets(g))) {
        setActionErr(t("pending.leak"));
        setErr(null);
      } else {
        setErr(null);
        setActionErr("");
      }
    } catch (e) {
      setErr(e);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  function openConfirm(kind: ConfirmKind, group: PendingGroup) {
    if (blocked) return;
    if (kind === "compact" && compactBlocked(group, Boolean(busy))) {
      setActionErr(t("pending.busy"));
      return;
    }
    setConfirm({ kind, group });
    setTyped("");
    setOk("");
    setActionErr("");
  }

  async function submitConfirm() {
    if (!confirm || blocked) return;
    const g = confirm.group;
    const name = typed.trim();
    if (!pendingConfirmMatch(name, g)) {
      setActionErr(t("pending.mismatch"));
      return;
    }
    const kind = confirm.kind;
    setBusy(`${kind}:${g.id}`);
    try {
      if (kind === "compact") {
        await pendingApi.compact(g.id, name);
        setOk(t("pending.compactOk"));
      } else {
        await pendingApi.clear(g.id, name);
        setOk(t("pending.clearOk"));
      }
      setActionErr("");
      setConfirm(null);
      setTyped("");
      await load();
    } catch (e) {
      const k = pendingWriteKind(e);
      if (k === "busy") setActionErr(t("pending.busy"));
      else if (k === "permission") setActionErr(kind === "compact" ? t("pending.unavailable") : t("pending.clearUnavailable"));
      else if (k === "mismatch") setActionErr(t("pending.mismatch"));
      else if (k === "missing") setActionErr(t("pending.missing"));
      else setActionErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  return (
    <PageChrome
      icon="hourglass"
      title={t("pending.title")}
      description={t("pending.desc")}
      primary={
        <Button icon="refresh" iconGesture variant="primary" onClick={() => void load()} disabled={loading || Boolean(busy)}>
          {t("common.refresh")}
        </Button>
      }
    >
      <Card>
        <CardHeader icon="inbox" title={t("pending.how")} />
        <p style={{ margin: 0, padding: "0 16px 14px", fontSize: 12.5, color: "var(--text-3)", maxWidth: 720 }}>
          {t("pending.howBody")}
        </p>
        <p style={{ margin: 0, padding: "0 16px 14px", fontSize: 12.5, color: "var(--text-3)", maxWidth: 720 }}>
          {t("pending.llmUnavailable")}
        </p>
      </Card>
      <PageStatus kind={state.kind} errorText={err ? formatPublicError(err) : ""} staleAt={formatStaleAt(loadedAt, locale)} onReload={() => void load()} />
      {actionErr ? <StatusLine kind="error">{actionErr}</StatusLine> : null}
      {inProgress && !blocked ? (
        <p role="status" style={{ margin: 0, fontSize: 12.5, color: "var(--text-2)" }}>
          {t("pending.compacting")}
        </p>
      ) : null}
      {ok && !actionErr ? (
        <p role="status" style={{ margin: 0, fontSize: 12.5, color: "var(--green)" }}>
          {ok}
        </p>
      ) : null}
      {confirm && !blocked ? (
        <Card>
          <CardHeader icon="lock" title={confirm.kind === "compact" ? t("pending.confirmCompactTitle") : t("pending.confirmClearTitle")} />
          <div style={{ padding: "0 16px 16px", display: "flex", flexDirection: "column", gap: 10 }}>
            <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-2)" }}>
              {t("pending.confirmPreview", { preview: previewLine(confirm.group, na) })}
            </p>
            <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>
              {confirm.kind === "compact" ? t("pending.confirmCompactHint") : t("pending.confirmClearHint")}
            </p>
            <input
              className="z-field"
              value={typed}
              onChange={(e) => setTyped(e.target.value)}
              placeholder={t("pending.confirmPlaceholder")}
              aria-label={t("pending.confirmPlaceholder")}
              autoComplete="off"
              spellCheck={false}
            />
            <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
              <Button
                variant={confirm.kind === "clear" ? "primary" : "accent"}
                disabled={!matched || Boolean(busy)}
                onClick={() => void submitConfirm()}
                style={confirm.kind === "clear" ? { background: "var(--red)", borderColor: "transparent" } : undefined}
              >
                {confirm.kind === "compact" ? t("pending.confirmCompact") : t("pending.confirmClear")}
              </Button>
              <Button
                variant="quiet"
                disabled={Boolean(busy)}
                onClick={() => {
                  setConfirm(null);
                  setTyped("");
                }}
              >
                {t("pending.cancel")}
              </Button>
            </div>
          </div>
        </Card>
      ) : null}
      <Card>
        <CardHeader icon="list" title={t("pending.list")} meta={metaN == null ? "—" : t("pending.meta", { n: metaN })} />
        <TableScroll>
          <div
            style={{
              display: "flex",
              padding: "8px 16px",
              borderBottom: "1px solid var(--border-soft)",
              fontSize: 10,
              fontWeight: 600,
              letterSpacing: ".4px",
              color: "var(--text-3)",
            }}
          >
            <span style={{ flex: 1.6 }}>{t("pending.col.channel")}</span>
            <span style={{ flex: 1.1 }}>{t("pending.col.count")}</span>
            <span style={{ flex: 1.1 }}>{t("pending.col.age")}</span>
            <span style={{ flex: 1.4 }}>{t("pending.col.agent")}</span>
            <span style={{ flex: 1.2 }}>{t("pending.col.status")}</span>
            <span style={{ flex: 1.8 }} />
          </div>
          {state.showEmpty ? <EmptyState>{t("pending.empty")}</EmptyState> : null}
          {state.showItems
            ? groups.map((g) => {
                const rowBusy = busy === `compact:${g.id}` || busy === `clear:${g.id}` || Boolean(g.compacting);
                const compactOff = blocked || compactBlocked(g, Boolean(busy));
                return (
                  <div
                    key={g.id}
                    style={{
                      display: "flex",
                      alignItems: "center",
                      padding: "11px 16px",
                      fontSize: 12.5,
                      borderBottom: "1px solid var(--border-soft)",
                      gap: 8,
                    }}
                  >
                    <span style={{ flex: 1.6, fontWeight: 600 }}>{groupLabel(g) || g.id}</span>
                    <span style={{ flex: 1.1, fontVariantNumeric: "tabular-nums" }}>{g.count}</span>
                    <span style={{ flex: 1.1, color: "var(--text-3)", fontVariantNumeric: "tabular-nums" }}>{formatAge(g.age_ms)}</span>
                    <span style={{ flex: 1.4, color: "var(--text-2)" }}>{agentLabel(g, na)}</span>
                    <span style={{ flex: 1.2 }}>
                      {g.compacting ? (
                        <Badge tone="warning">{t("pending.compacting")}</Badge>
                      ) : g.compacted ? (
                        <Badge tone="accent">{t("pending.compacted")}</Badge>
                      ) : (
                        <Badge tone="neutral">{g.count}</Badge>
                      )}
                    </span>
                    <span style={{ flex: 1.8, display: "flex", gap: 6, justifyContent: "flex-end", flexWrap: "wrap" }}>
                      <Button variant="quiet" disabled={compactOff} onClick={() => openConfirm("compact", g)}>
                        {t("pending.compact")}
                      </Button>
                      <Button
                        variant="quiet"
                        disabled={blocked || rowBusy}
                        onClick={() => openConfirm("clear", g)}
                        style={{ color: "var(--red)" }}
                      >
                        {t("pending.clear")}
                      </Button>
                    </span>
                  </div>
                );
              })
            : null}
        </TableScroll>
      </Card>
    </PageChrome>
  );
}
