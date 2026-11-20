import { useEffect, useState } from "react";
import { nodesApi, type NodeDevice } from "../api/nodes";
import { asPublic, formatWhen, nodeConfirmMatch, nodeInventoryCount, nodeLabel, publicHasSecrets } from "../api/nodes-ops";
import { classifyPageState, formatStaleAt, inventoryBlocksMutation, listMetaCount } from "../api/page-state";
import { useI18n } from "../i18n";
import { Button } from "../ui/Button";
import { Card, CardHeader } from "../ui/Card";
import { DataTable, THead, Th, Td, Tr } from "../ui/DataTable";
import { EmptyState } from "../ui/EmptyState";
import { PageChrome } from "../ui/PageChrome";
import { PageStatus } from "../ui/PageStatus";
import { StatusBadge, type Status } from "../ui/StatusBadge";
import { StatusLine, formatPublicError } from "../ui/StatusLine";
import { fieldClass } from "../ui/field";

type ActionKind = "approve" | "deny" | "revoke";

export function NodesPage() {
  const { t, locale } = useI18n();
  const [pending, setPending] = useState<NodeDevice[]>([]);
  const [paired, setPaired] = useState<NodeDevice[]>([]);
  const [loading, setLoading] = useState(true);
  const [loaded, setLoaded] = useState(false);
  const [loadedAt, setLoadedAt] = useState<string | null>(null);
  const [err, setErr] = useState<unknown>(null);
  const [actionErr, setActionErr] = useState("");
  const [ok, setOk] = useState("");
  const [busy, setBusy] = useState("");
  const [confirm, setConfirm] = useState<{ kind: ActionKind; row: NodeDevice } | null>(null);
  const [typed, setTyped] = useState("");
  const na = t("nodes.na");
  const state = classifyPageState({
    loading,
    loaded,
    error: err,
    itemCount: nodeInventoryCount(pending, paired),
    keepStale: loaded && nodeInventoryCount(pending, paired) > 0,
  });
  const blocked = inventoryBlocksMutation(state.kind);
  const pendingMeta = listMetaCount(state.kind, pending.length);
  const pairedMeta = listMetaCount(state.kind, paired.length);
  const matched = confirm ? nodeConfirmMatch(typed, confirm.row) : false;

  async function load() {
    setLoading(true);
    try {
      const j = await nodesApi.list();
      const nextPending = asPublic(j.pending);
      const nextPaired = asPublic(j.paired);
      setPending(nextPending);
      setPaired(nextPaired);
      setLoaded(true);
      setLoadedAt(new Date().toISOString());
      const leak =
        nextPending.some((row) => publicHasSecrets(row)) ||
        nextPaired.some((row) => publicHasSecrets(row)) ||
        (j.pending || []).some((row) => publicHasSecrets(row)) ||
        (j.paired || []).some((row) => publicHasSecrets(row));
      setActionErr(leak ? t("nodes.leak") : "");
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

  function openConfirm(kind: ActionKind, row: NodeDevice) {
    if (blocked) return;
    setConfirm({ kind, row });
    setTyped("");
    setOk("");
    setActionErr("");
  }

  async function submitConfirm() {
    if (!confirm || blocked) return;
    if (!nodeConfirmMatch(typed, confirm.row)) {
      setActionErr(t("nodes.mismatch"));
      return;
    }
    const name = typed.trim();
    const kind = confirm.kind;
    setBusy(`${kind}:${confirm.row.id}`);
    try {
      if (kind === "approve") await nodesApi.approve(confirm.row.id, name);
      else if (kind === "deny") await nodesApi.deny(confirm.row.id, name);
      else await nodesApi.revoke(confirm.row.id, name);
      setOk(kind === "approve" ? t("nodes.approveOk") : kind === "deny" ? t("nodes.denyOk") : t("nodes.revokeOk"));
      setActionErr("");
      setConfirm(null);
      setTyped("");
      await load();
    } catch (e) {
      setActionErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  function healthStatus(h: string): Status {
    if (h === "ok") return "success";
    if (h === "stale" || h === "pending") return "warning";
    if (h === "expired") return "error";
    return "default";
  }

  function healthLabel(h: string): string {
    if (h === "ok") return t("nodes.health.ok");
    if (h === "stale") return t("nodes.health.stale");
    if (h === "expired") return t("nodes.health.expired");
    if (h === "pending") return t("nodes.health.pending");
    return h || na;
  }

  function confirmTitle(kind: ActionKind): string {
    if (kind === "approve") return t("nodes.confirmApproveTitle");
    if (kind === "deny") return t("nodes.confirmDenyTitle");
    return t("nodes.confirmRevokeTitle");
  }

  function confirmPreview(kind: ActionKind, row: NodeDevice): string {
    const name = nodeLabel(row);
    if (kind === "approve") return t("nodes.confirmApprovePreview", { name });
    if (kind === "deny") return t("nodes.confirmDenyPreview", { name });
    return t("nodes.confirmRevokePreview", { name });
  }

  function confirmAction(kind: ActionKind): string {
    if (kind === "approve") return t("nodes.confirmApprove");
    if (kind === "deny") return t("nodes.confirmDeny");
    return t("nodes.confirmRevoke");
  }

  function renderRows(rows: NodeDevice[], kind: "pending" | "paired") {
    if (!state.showItems) return null;
    return rows.map((row) => {
      const rowBusy = busy.endsWith(":" + row.id);
      return (
        <Tr key={row.id}>
          <Td className="font-semibold">{nodeLabel(row)}</Td>
          <Td className="text-muted-foreground">{row.kind || na}</Td>
          <Td className="tabular-nums text-muted-foreground">
            {kind === "pending" ? formatWhen(row.expires_at, na) : formatWhen(row.last_seen, na)}
          </Td>
          <Td>
            <StatusBadge status={healthStatus(row.health)} label={healthLabel(row.health)} />
          </Td>
          <Td>
            <div className="flex flex-wrap justify-end gap-2">
              {kind === "pending" ? (
                <>
                  <Button variant="outline" size="sm" disabled={blocked || rowBusy || row.health === "expired"} onClick={() => openConfirm("approve", row)}>
                    {t("nodes.approve")}
                  </Button>
                  <Button variant="ghost" size="sm" className="text-destructive" disabled={blocked || rowBusy} onClick={() => openConfirm("deny", row)}>
                    {t("nodes.deny")}
                  </Button>
                </>
              ) : (
                <Button variant="ghost" size="sm" className="text-destructive" disabled={blocked || rowBusy} onClick={() => openConfirm("revoke", row)}>
                  {t("nodes.revoke")}
                </Button>
              )}
            </div>
          </Td>
        </Tr>
      );
    });
  }

  return (
    <PageChrome
      icon="plug"
      title={t("nodes.title")}
      description={t("nodes.desc")}
      refresh={
        <Button icon="refresh" iconGesture variant="outline" onClick={() => void load()} disabled={loading || Boolean(busy)}>
          {t("common.refresh")}
        </Button>
      }
    >
      <Card>
        <CardHeader icon="lock" title={t("nodes.how")} />
        <p className="max-w-xl px-4 pb-4 text-sm text-muted-foreground">{t("nodes.howBody")}</p>
      </Card>
      <PageStatus kind={state.kind} errorText={err ? formatPublicError(err) : ""} staleAt={formatStaleAt(loadedAt, locale)} onReload={() => void load()} />
      {actionErr ? <StatusLine kind="error">{actionErr}</StatusLine> : null}
      {ok && !actionErr ? (
        <p role="status" className="text-sm text-success">
          {ok}
        </p>
      ) : null}
      {confirm && !blocked ? (
        <Card>
          <CardHeader icon="lock" title={confirmTitle(confirm.kind)} />
          <div className="flex flex-col gap-3 p-4">
            <p className="text-sm">{confirmPreview(confirm.kind, confirm.row)}</p>
            <p className="text-sm text-muted-foreground">{t("nodes.confirmHint")}</p>
            <input
              className={fieldClass}
              value={typed}
              onChange={(e) => setTyped(e.target.value)}
              placeholder={t("nodes.confirmPlaceholder")}
              aria-label={t("nodes.confirmPlaceholder")}
              autoComplete="off"
              spellCheck={false}
            />
            <div className="flex flex-wrap gap-2">
              <Button
                variant={confirm.kind === "approve" ? "default" : "destructive"}
                disabled={!matched || Boolean(busy)}
                onClick={() => void submitConfirm()}
              >
                {confirmAction(confirm.kind)}
              </Button>
              <Button
                variant="outline"
                disabled={Boolean(busy)}
                onClick={() => {
                  setConfirm(null);
                  setTyped("");
                }}
              >
                {t("nodes.cancel")}
              </Button>
            </div>
          </div>
        </Card>
      ) : null}
      <div>
        <div className="flex items-center justify-between">
          <h2 className="text-sm font-semibold">{t("nodes.pending")}</h2>
          <span className="text-xs text-muted-foreground">{pendingMeta == null ? "—" : t("nodes.pending.meta", { n: pendingMeta })}</span>
        </div>
        {state.showEmpty || (state.showItems && pending.length === 0) ? (
          <EmptyState icon="plug" title={t("nodes.pending.empty")} />
        ) : null}
        {state.showItems && pending.length > 0 ? (
          <DataTable minWidth="720px">
            <THead>
              <Th>{t("nodes.col.device")}</Th>
              <Th>{t("nodes.col.kind")}</Th>
              <Th>{t("nodes.col.expires")}</Th>
              <Th>{t("nodes.col.health")}</Th>
              <Th className="text-right" />
            </THead>
            <tbody>{renderRows(pending, "pending")}</tbody>
          </DataTable>
        ) : null}
      </div>
      <div>
        <div className="flex items-center justify-between">
          <h2 className="text-sm font-semibold">{t("nodes.paired")}</h2>
          <span className="text-xs text-muted-foreground">{pairedMeta == null ? "—" : t("nodes.paired.meta", { n: pairedMeta })}</span>
        </div>
        {state.showEmpty || (state.showItems && paired.length === 0) ? (
          <EmptyState icon="plug" title={t("nodes.paired.empty")} />
        ) : null}
        {state.showItems && paired.length > 0 ? (
          <DataTable minWidth="720px">
            <THead>
              <Th>{t("nodes.col.device")}</Th>
              <Th>{t("nodes.col.kind")}</Th>
              <Th>{t("nodes.col.seen")}</Th>
              <Th>{t("nodes.col.health")}</Th>
              <Th className="text-right" />
            </THead>
            <tbody>{renderRows(paired, "paired")}</tbody>
          </DataTable>
        ) : null}
      </div>
    </PageChrome>
  );
}
