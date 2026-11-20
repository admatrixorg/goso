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
import { Button } from "../ui/Button";
import { Card, CardHeader } from "../ui/Card";
import { DataTable, THead, Th, Td, Tr } from "../ui/DataTable";
import { EmptyState } from "../ui/EmptyState";
import { PageChrome } from "../ui/PageChrome";
import { PageStatus } from "../ui/PageStatus";
import { StatusBadge } from "../ui/StatusBadge";
import { StatusLine, formatPublicError } from "../ui/StatusLine";
import { fieldClass } from "../ui/field";

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
      icon="inbox"
      title={t("pending.title")}
      description={t("pending.desc")}
      refresh={
        <Button icon="refresh" iconGesture variant="outline" size="sm" onClick={() => void load()} disabled={loading || Boolean(busy)}>
          {t("common.refresh")}
        </Button>
      }
    >
      <Card>
        <CardHeader icon="inbox" title={t("pending.how")} />
        <p className="max-w-xl px-4 pb-3 text-sm text-muted-foreground">{t("pending.howBody")}</p>
        <p className="max-w-xl px-4 pb-4 text-sm text-muted-foreground">{t("pending.llmUnavailable")}</p>
      </Card>
      <PageStatus kind={state.kind} errorText={err ? formatPublicError(err) : ""} staleAt={formatStaleAt(loadedAt, locale)} onReload={() => void load()} />
      {actionErr ? <StatusLine kind="error">{actionErr}</StatusLine> : null}
      {inProgress && !blocked ? (
        <p role="status" className="text-sm text-muted-foreground">{t("pending.compacting")}</p>
      ) : null}
      {ok && !actionErr ? (
        <p role="status" className="text-sm text-success">{ok}</p>
      ) : null}
      {confirm && !blocked ? (
        <Card>
          <CardHeader icon="lock" title={confirm.kind === "compact" ? t("pending.confirmCompactTitle") : t("pending.confirmClearTitle")} />
          <div className="flex flex-col gap-3 px-4 pb-4">
            <p className="text-sm text-foreground">{t("pending.confirmPreview", { preview: previewLine(confirm.group, na) })}</p>
            <p className="text-sm text-muted-foreground">
              {confirm.kind === "compact" ? t("pending.confirmCompactHint") : t("pending.confirmClearHint")}
            </p>
            <input
              className={fieldClass}
              value={typed}
              onChange={(e) => setTyped(e.target.value)}
              placeholder={t("pending.confirmPlaceholder")}
              aria-label={t("pending.confirmPlaceholder")}
              autoComplete="off"
              spellCheck={false}
            />
            <div className="flex flex-wrap gap-2">
              <Button
                variant={confirm.kind === "clear" ? "destructive" : "default"}
                disabled={!matched || Boolean(busy)}
                onClick={() => void submitConfirm()}
              >
                {confirm.kind === "compact" ? t("pending.confirmCompact") : t("pending.confirmClear")}
              </Button>
              <Button
                variant="outline"
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
      {state.showEmpty ? <EmptyState icon="inbox" title={t("pending.empty")} /> : null}
      {state.showItems ? (
        <DataTable minWidth="720px">
          <THead>
            <Th>{t("pending.col.channel")}</Th>
            <Th>{t("pending.col.count")}</Th>
            <Th>{t("pending.col.age")}</Th>
            <Th>{t("pending.col.agent")}</Th>
            <Th>{t("pending.col.status")}</Th>
            <Th className="text-right" />
          </THead>
          <tbody>
            {groups.map((g) => {
              const rowBusy = busy === ("compact:" + g.id) || busy === ("clear:" + g.id) || Boolean(g.compacting);
              const compactOff = blocked || compactBlocked(g, Boolean(busy));
              return (
                <Tr key={g.id}>
                  <Td className="font-semibold">{groupLabel(g) || g.id}</Td>
                  <Td className="tabular-nums">{g.count}</Td>
                  <Td className="tabular-nums text-muted-foreground">{formatAge(g.age_ms)}</Td>
                  <Td className="text-muted-foreground">{agentLabel(g, na)}</Td>
                  <Td>
                    {g.compacting ? (
                      <StatusBadge status="warning" label={t("pending.compacting")} />
                    ) : g.compacted ? (
                      <StatusBadge status="info" label={t("pending.compacted")} />
                    ) : (
                      <StatusBadge status="default" label={String(g.count)} />
                    )}
                  </Td>
                  <Td>
                    <div className="flex flex-wrap justify-end gap-2">
                      <Button variant="outline" size="sm" disabled={compactOff} onClick={() => openConfirm("compact", g)}>
                        {t("pending.compact")}
                      </Button>
                      <Button variant="ghost" size="sm" className="text-destructive" disabled={blocked || rowBusy} onClick={() => openConfirm("clear", g)}>
                        {t("pending.clear")}
                      </Button>
                    </div>
                  </Td>
                </Tr>
              );
            })}
          </tbody>
        </DataTable>
      ) : null}
    </PageChrome>
  );
}
