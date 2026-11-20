import { useEffect, useState } from "react";
import { approvalsApi, type Approval } from "../api/approvals";
import {
  POLL_MS,
  approvalLabel,
  canResolve,
  formatWhen,
  listHasSecrets,
  publicHasSecrets,
} from "../api/approvals-ops";
import { classifyPageState, formatStaleAt, inventoryBlocksMutation, listMetaCount } from "../api/page-state";
import { useI18n, type MsgKey } from "../i18n";
import { Button } from "../ui/Button";
import { Card, CardHeader } from "../ui/Card";
import { DataTable, THead, Th, Td, Tr } from "../ui/DataTable";
import { EmptyState } from "../ui/EmptyState";
import { PageChrome } from "../ui/PageChrome";
import { PageStatus } from "../ui/PageStatus";
import { StatusBadge, type Status } from "../ui/StatusBadge";
import { StatusLine, formatPublicError } from "../ui/StatusLine";
import { fieldClass } from "../ui/field";

type ActionKind = "approve" | "deny";

function statusStatus(st: string): Status {
  if (st === "approved") return "success";
  if (st === "pending") return "warning";
  if (st === "expired" || st === "rejected") return "error";
  return "default";
}

function statusKey(st: string): MsgKey {
  if (st === "approved") return "approvals.status.approved";
  if (st === "rejected") return "approvals.status.denied";
  if (st === "expired") return "approvals.status.expired";
  return "approvals.status.pending";
}

function riskStatus(risk: string): Status {
  if (risk === "high") return "error";
  if (risk === "low") return "success";
  return "warning";
}

function riskKey(risk: string): MsgKey {
  if (risk === "high") return "approvals.risk.high";
  if (risk === "low") return "approvals.risk.low";
  return "approvals.risk.medium";
}

export function ApprovalsPage() {
  const { t, locale } = useI18n();
  const [rows, setRows] = useState<Approval[]>([]);
  const [open, setOpen] = useState("");
  const [loading, setLoading] = useState(true);
  const [loaded, setLoaded] = useState(false);
  const [loadedAt, setLoadedAt] = useState<string | null>(null);
  const [err, setErr] = useState<unknown>(null);
  const [actionErr, setActionErr] = useState("");
  const [ok, setOk] = useState("");
  const [busy, setBusy] = useState("");
  const [confirm, setConfirm] = useState<{ kind: ActionKind; row: Approval } | null>(null);
  const [reason, setReason] = useState("");
  const na = t("approvals.na");

  const state = classifyPageState({
    loading,
    loaded,
    error: err,
    itemCount: rows.length,
    keepStale: loaded && rows.length > 0,
  });
  const blocked = inventoryBlocksMutation(state.kind);
  const metaN = listMetaCount(state.kind, rows.length);
  const shown = state.showItems ? rows : [];
  const selected = shown.find((r) => r.id === open) ?? null;

  async function load(quiet = false) {
    if (!quiet) setLoading(true);
    try {
      const j = await approvalsApi.list();
      setRows(j.approvals);
      setLoaded(true);
      setLoadedAt(new Date().toISOString());
      setErr(null);
      setActionErr(listHasSecrets(j) ? t("approvals.leak") : "");
    } catch (e) {
      setErr(e);
    } finally {
      if (!quiet) setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  useEffect(() => {
    const id = window.setInterval(() => {
      void load(true);
    }, POLL_MS);
    return () => window.clearInterval(id);
  }, []);

  function openConfirm(kind: ActionKind, row: Approval) {
    if (blocked || !canResolve(row)) return;
    setConfirm({ kind, row });
    setReason("");
    setOk("");
    setActionErr("");
  }

  async function submitConfirm() {
    if (!confirm || blocked) return;
    if (confirm.kind === "deny" && !reason.trim()) {
      setActionErr(t("approvals.reasonRequired"));
      return;
    }
    const id = confirm.row.id;
    const kind = confirm.kind;
    setBusy(`${kind}:${id}`);
    try {
      const out = await approvalsApi.decide(id, kind, kind === "deny" ? reason.trim() : undefined);
      if (publicHasSecrets(out)) {
        setActionErr(t("approvals.leak"));
      } else {
        setOk(kind === "approve" ? t("approvals.approveOk") : t("approvals.denyOk"));
        setActionErr("");
      }
      setConfirm(null);
      setReason("");
      await load(true);
    } catch (e) {
      setActionErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  return (
    <PageChrome
      icon="shield-check"
      title={t("approvals.title")}
      description={t("approvals.desc")}
      refresh={
        <Button icon="refresh" iconGesture variant="outline" onClick={() => void load()} disabled={loading || Boolean(busy)}>
          {t("common.refresh")}
        </Button>
      }
    >
      <Card>
        <CardHeader icon="lock" title={t("approvals.how")} />
        <p className="max-w-xl p-4 text-sm text-muted-foreground">{t("approvals.howBody")}</p>
      </Card>
      <PageStatus
        kind={state.kind}
        errorText={err ? formatPublicError(err) : ""}
        staleAt={formatStaleAt(loadedAt, locale)}
        onReload={() => void load()}
      />
      {actionErr ? <StatusLine kind="error">{actionErr}</StatusLine> : null}
      {ok && !actionErr ? (
        <p role="status" className="text-sm text-success">
          {ok}
        </p>
      ) : null}
      {confirm && !blocked ? (
        <Card>
          <CardHeader icon="lock" title={confirm.kind === "approve" ? t("approvals.confirmApproveTitle") : t("approvals.confirmDenyTitle")} />
          <div className="flex flex-col gap-3 p-4">
            <p className="text-sm">
              {confirm.kind === "approve"
                ? t("approvals.confirmApprovePreview", { name: approvalLabel(confirm.row) })
                : t("approvals.confirmDenyPreview", { name: approvalLabel(confirm.row) })}
            </p>
            {confirm.kind === "deny" ? (
              <>
                <p className="text-sm text-muted-foreground">{t("approvals.reasonHint")}</p>
                <textarea
                  className={`${fieldClass} h-auto min-h-[72px] py-2`}
                  value={reason}
                  onChange={(e) => setReason(e.target.value)}
                  placeholder={t("approvals.reasonPlaceholder")}
                  aria-label={t("approvals.reasonPlaceholder")}
                  rows={3}
                />
              </>
            ) : null}
            <div className="flex flex-wrap gap-2">
              <Button
                variant={confirm.kind === "deny" ? "destructive" : "default"}
                disabled={Boolean(busy) || (confirm.kind === "deny" && !reason.trim())}
                onClick={() => void submitConfirm()}
              >
                {confirm.kind === "approve" ? t("approvals.confirmApprove") : t("approvals.confirmDeny")}
              </Button>
              <Button
                variant="outline"
                disabled={Boolean(busy)}
                onClick={() => {
                  setConfirm(null);
                  setReason("");
                }}
              >
                {t("approvals.cancel")}
              </Button>
            </div>
          </div>
        </Card>
      ) : null}
      {state.showEmpty ? <EmptyState icon="shield-check" title={t("approvals.empty")} data-page-state="empty" /> : null}
      {state.showItems ? (
        <DataTable minWidth="880px">
          <THead>
            <Th>{t("approvals.col.requester")}</Th>
            <Th>{t("approvals.col.agent")}</Th>
            <Th>{t("approvals.col.tool")}</Th>
            <Th>{t("approvals.col.preview")}</Th>
            <Th>{t("approvals.col.risk")}</Th>
            <Th>{t("approvals.col.expiry")}</Th>
            <Th className="text-right" />
          </THead>
          <tbody>
            {shown.map((row) => {
              const rowBusy = busy.endsWith(":" + row.id);
              const expired = row.status === "expired" || row.stale;
              const resolved = row.status === "approved" || row.status === "rejected";
              const act = canResolve(row) && !blocked;
              return (
                <Tr key={row.id} selected={open === row.id} onClick={() => setOpen(open === row.id ? "" : row.id)}>
                  <Td className="font-semibold">{row.requester || na}</Td>
                  <Td className="text-muted-foreground">{row.agent_id || na}</Td>
                  <Td>{approvalLabel(row)}</Td>
                  <Td className="max-w-[220px] truncate text-muted-foreground">{row.arg_preview || "{}"}</Td>
                  <Td>
                    <StatusBadge status={riskStatus(row.risk)} label={t(riskKey(row.risk))} />
                  </Td>
                  <Td className="tabular-nums text-muted-foreground">{formatWhen(row.expires_at, na)}</Td>
                  <Td onClick={(e) => e.stopPropagation()}>
                    <div className="flex flex-wrap items-center justify-end gap-2">
                      <StatusBadge status={statusStatus(row.status)} label={t(statusKey(row.status))} />
                      {expired && !resolved ? <StatusBadge status="warning" label={t("approvals.status.expired")} /> : null}
                      <Button variant="outline" size="sm" disabled={rowBusy || !act} onClick={() => openConfirm("approve", row)}>
                        {t("approvals.approve")}
                      </Button>
                      <Button variant="ghost" size="sm" className="text-destructive" disabled={rowBusy || !act} onClick={() => openConfirm("deny", row)}>
                        {t("approvals.deny")}
                      </Button>
                    </div>
                  </Td>
                </Tr>
              );
            })}
          </tbody>
        </DataTable>
      ) : null}
      {state.showItems ? (
        <p className="text-xs text-muted-foreground">{metaN == null ? "—" : t("approvals.meta", { n: metaN })}</p>
      ) : null}
      {selected ? (
        <Card>
          <CardHeader icon="list" title={t("approvals.detail")} />
          <div className="flex flex-col gap-2 p-4 text-sm">
            <p className="m-0">
              <span className="text-muted-foreground">{t("approvals.col.requester")}: </span>
              {selected.requester || na}
            </p>
            <p className="m-0">
              <span className="text-muted-foreground">{t("approvals.col.agent")}: </span>
              {selected.agent_id || na}
            </p>
            <p className="m-0">
              <span className="text-muted-foreground">{t("approvals.col.tool")}: </span>
              {approvalLabel(selected)}
            </p>
            <p className="m-0 break-all">
              <span className="text-muted-foreground">{t("approvals.col.preview")}: </span>
              {selected.arg_preview}
            </p>
            <p className="m-0">
              <span className="text-muted-foreground">{t("approvals.col.risk")}: </span>
              {t(riskKey(selected.risk))}
            </p>
            <p className="m-0">
              <span className="text-muted-foreground">{t("approvals.col.expiry")}: </span>
              {formatWhen(selected.expires_at, na)}
            </p>
            {selected.status === "approved" || selected.status === "rejected" ? (
              <p className="m-0 text-muted-foreground">{t("approvals.resolvedHint")}</p>
            ) : null}
            {selected.stale || selected.status === "expired" ? (
              <p className="m-0 text-warning">{t("approvals.expiredHint")}</p>
            ) : null}
          </div>
        </Card>
      ) : null}
    </PageChrome>
  );
}
