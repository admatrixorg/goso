import { useEffect, useRef, useState } from "react";
import { approvalsApi, type Approval } from "../api/approvals";
import {
  POLL_MS,
  STALE_MS,
  approvalLabel,
  canResolve,
  formatWhen,
  listHasSecrets,
  publicHasSecrets,
} from "../api/approvals-ops";
import { useI18n, type MsgKey } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { SectionHeader } from "../ui/SectionHeader";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

type ActionKind = "approve" | "deny";

function statusTone(st: string): "positive" | "warning" | "critical" | "neutral" {
  if (st === "approved") return "positive";
  if (st === "pending") return "warning";
  if (st === "expired" || st === "rejected") return "critical";
  return "neutral";
}

function statusKey(st: string): MsgKey {
  if (st === "approved") return "approvals.status.approved";
  if (st === "rejected") return "approvals.status.denied";
  if (st === "expired") return "approvals.status.expired";
  return "approvals.status.pending";
}

function riskTone(risk: string): "positive" | "warning" | "critical" | "neutral" {
  if (risk === "high") return "critical";
  if (risk === "low") return "positive";
  return "warning";
}

function riskKey(risk: string): MsgKey {
  if (risk === "high") return "approvals.risk.high";
  if (risk === "low") return "approvals.risk.low";
  return "approvals.risk.medium";
}

export function ApprovalsPage() {
  const { t } = useI18n();
  const [rows, setRows] = useState<Approval[]>([]);
  const [open, setOpen] = useState("");
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState("");
  const [ok, setOk] = useState("");
  const [busy, setBusy] = useState("");
  const [stale, setStale] = useState(false);
  const [confirm, setConfirm] = useState<{ kind: ActionKind; row: Approval } | null>(null);
  const [reason, setReason] = useState("");
  const lastOk = useRef(0);
  const na = t("approvals.na");

  async function load(quiet = false) {
    if (!quiet) setLoading(true);
    try {
      const j = await approvalsApi.list();
      setRows(j.approvals);
      if (listHasSecrets(j)) setErr(t("approvals.leak"));
      else setErr("");
      lastOk.current = Date.now();
      setStale(false);
    } catch (e) {
      setErr(formatPublicError(e));
      if (Date.now() - lastOk.current > STALE_MS) setStale(true);
    } finally {
      if (!quiet) setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  useEffect(() => {
    const id = window.setInterval(() => {
      if (Date.now() - lastOk.current > STALE_MS) setStale(true);
      void load(true);
    }, POLL_MS);
    return () => window.clearInterval(id);
  }, []);

  function openConfirm(kind: ActionKind, row: Approval) {
    setConfirm({ kind, row });
    setReason("");
    setOk("");
    setErr("");
  }

  async function submitConfirm() {
    if (!confirm) return;
    if (confirm.kind === "deny" && !reason.trim()) {
      setErr(t("approvals.reasonRequired"));
      return;
    }
    const id = confirm.row.id;
    const kind = confirm.kind;
    setBusy(`${kind}:${id}`);
    try {
      const out = await approvalsApi.decide(id, kind, kind === "deny" ? reason.trim() : undefined);
      if (publicHasSecrets(out)) {
        setErr(t("approvals.leak"));
      } else {
        setOk(kind === "approve" ? t("approvals.approveOk") : t("approvals.denyOk"));
        setErr("");
      }
      setConfirm(null);
      setReason("");
      await load(true);
    } catch (e) {
      const msg = formatPublicError(e);
      setErr(msg);
    } finally {
      setBusy("");
    }
  }

  const selected = rows.find((r) => r.id === open) ?? null;

  return (
    <div style={{ padding: "14px 22px 40px", display: "flex", flexDirection: "column", gap: 14 }}>
      <SectionHeader
        icon="shield"
        title={t("approvals.title")}
        description={t("approvals.desc")}
        actions={
          <Button icon="refresh" iconGesture onClick={() => void load()} disabled={loading || Boolean(busy)}>
            {t("common.refresh")}
          </Button>
        }
      />
      <Card>
        <CardHeader icon="lock" title={t("approvals.how")} />
        <p style={{ margin: 0, padding: "0 16px 14px", fontSize: 12.5, color: "var(--text-3)", maxWidth: 720 }}>
          {t("approvals.howBody")}
        </p>
      </Card>
      {loading ? <StatusLine kind="loading" /> : null}
      {err ? <StatusLine kind="error">{err}</StatusLine> : null}
      {stale && !loading ? (
        <p role="status" style={{ margin: 0, fontSize: 12.5, color: "var(--orange, #b45309)" }}>
          {t("approvals.stale")}
        </p>
      ) : null}
      {ok && !err ? (
        <p role="status" style={{ margin: 0, fontSize: 12.5, color: "var(--green)" }}>
          {ok}
        </p>
      ) : null}
      {confirm ? (
        <Card>
          <CardHeader icon="lock" title={confirm.kind === "approve" ? t("approvals.confirmApproveTitle") : t("approvals.confirmDenyTitle")} />
          <div style={{ padding: "0 16px 16px", display: "flex", flexDirection: "column", gap: 10 }}>
            <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-2)" }}>
              {confirm.kind === "approve"
                ? t("approvals.confirmApprovePreview", { name: approvalLabel(confirm.row) })
                : t("approvals.confirmDenyPreview", { name: approvalLabel(confirm.row) })}
            </p>
            {confirm.kind === "deny" ? (
              <>
                <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("approvals.reasonHint")}</p>
                <textarea
                  className="z-field"
                  value={reason}
                  onChange={(e) => setReason(e.target.value)}
                  placeholder={t("approvals.reasonPlaceholder")}
                  aria-label={t("approvals.reasonPlaceholder")}
                  rows={3}
                  style={{ resize: "vertical", minHeight: 72 }}
                />
              </>
            ) : null}
            <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
              <Button
                variant="accent"
                disabled={Boolean(busy) || (confirm.kind === "deny" && !reason.trim())}
                onClick={() => void submitConfirm()}
              >
                {confirm.kind === "approve" ? t("approvals.confirmApprove") : t("approvals.confirmDeny")}
              </Button>
              <Button
                variant="quiet"
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
      <Card>
        <CardHeader icon="inbox" title={t("approvals.inbox")} meta={t("approvals.meta", { n: rows.length })} />
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
            <span style={{ flex: 1.3 }}>{t("approvals.col.requester")}</span>
            <span style={{ flex: 1.1 }}>{t("approvals.col.agent")}</span>
            <span style={{ flex: 1.3 }}>{t("approvals.col.tool")}</span>
            <span style={{ flex: 1.6 }}>{t("approvals.col.preview")}</span>
            <span style={{ flex: 0.8 }}>{t("approvals.col.risk")}</span>
            <span style={{ flex: 1.2 }}>{t("approvals.col.expiry")}</span>
            <span style={{ flex: 1.6 }} />
          </div>
          {!loading && rows.length === 0 ? <EmptyState>{t("approvals.empty")}</EmptyState> : null}
          {rows.map((row) => {
            const rowBusy = busy.endsWith(":" + row.id);
            const act = canResolve(row);
            return (
              <div key={row.id} style={{ borderBottom: "1px solid var(--border-soft)" }}>
                <div
                  style={{
                    display: "flex",
                    alignItems: "center",
                    padding: "11px 16px",
                    fontSize: 12.5,
                    gap: 8,
                  }}
                >
                  <button
                    type="button"
                    onClick={() => setOpen(open === row.id ? "" : row.id)}
                    aria-expanded={open === row.id}
                    style={{
                      flex: 1.3,
                      fontWeight: 600,
                      background: "none",
                      border: 0,
                      color: "inherit",
                      textAlign: "left",
                      cursor: "pointer",
                      fontFamily: "inherit",
                      fontSize: "inherit",
                      padding: 0,
                    }}
                  >
                    {row.requester || na}
                  </button>
                  <span style={{ flex: 1.1, color: "var(--text-2)" }}>{row.agent_id || na}</span>
                  <span style={{ flex: 1.3 }}>{approvalLabel(row)}</span>
                  <span style={{ flex: 1.6, color: "var(--text-3)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                    {row.arg_preview || "{}"}
                  </span>
                  <span style={{ flex: 0.8 }}>
                    <Badge tone={riskTone(row.risk)}>{t(riskKey(row.risk))}</Badge>
                  </span>
                  <span style={{ flex: 1.2, color: "var(--text-3)", fontVariantNumeric: "tabular-nums" }}>
                    {formatWhen(row.expires_at, na)}
                  </span>
                  <span style={{ flex: 1.6, display: "flex", gap: 6, justifyContent: "flex-end", flexWrap: "wrap" }}>
                    <Badge tone={statusTone(row.status)}>{t(statusKey(row.status))}</Badge>
                    <Button variant="quiet" disabled={rowBusy || !act} onClick={() => openConfirm("approve", row)}>
                      {t("approvals.approve")}
                    </Button>
                    <Button variant="quiet" disabled={rowBusy || !act} onClick={() => openConfirm("deny", row)}>
                      {t("approvals.deny")}
                    </Button>
                  </span>
                </div>
              </div>
            );
          })}
        </TableScroll>
      </Card>
      {selected ? (
        <Card>
          <CardHeader icon="list" title={t("approvals.detail")} />
          <div style={{ padding: "0 16px 16px", display: "flex", flexDirection: "column", gap: 6, fontSize: 12.5 }}>
            <p style={{ margin: 0 }}>
              <span style={{ color: "var(--text-3)" }}>{t("approvals.col.requester")}: </span>
              {selected.requester || na}
            </p>
            <p style={{ margin: 0 }}>
              <span style={{ color: "var(--text-3)" }}>{t("approvals.col.agent")}: </span>
              {selected.agent_id || na}
            </p>
            <p style={{ margin: 0 }}>
              <span style={{ color: "var(--text-3)" }}>{t("approvals.col.tool")}: </span>
              {approvalLabel(selected)}
            </p>
            <p style={{ margin: 0, overflowWrap: "anywhere" }}>
              <span style={{ color: "var(--text-3)" }}>{t("approvals.col.preview")}: </span>
              {selected.arg_preview}
            </p>
            <p style={{ margin: 0 }}>
              <span style={{ color: "var(--text-3)" }}>{t("approvals.col.risk")}: </span>
              {t(riskKey(selected.risk))}
            </p>
            <p style={{ margin: 0 }}>
              <span style={{ color: "var(--text-3)" }}>{t("approvals.col.expiry")}: </span>
              {formatWhen(selected.expires_at, na)}
            </p>
            {selected.stale || selected.status === "expired" ? (
              <p style={{ margin: 0, color: "var(--orange, #b45309)" }}>{t("approvals.expiredHint")}</p>
            ) : null}
          </div>
        </Card>
      ) : null}
    </div>
  );
}
