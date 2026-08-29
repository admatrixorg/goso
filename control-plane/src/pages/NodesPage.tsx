import { useEffect, useState } from "react";
import { nodesApi, type NodeDevice } from "../api/nodes";
import { asPublic, formatWhen, nodeConfirmMatch, nodeLabel, publicHasSecrets } from "../api/nodes-ops";
import { useI18n } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { SectionHeader } from "../ui/SectionHeader";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

type ActionKind = "approve" | "deny" | "revoke";

export function NodesPage() {
  const { t } = useI18n();
  const [pending, setPending] = useState<NodeDevice[]>([]);
  const [paired, setPaired] = useState<NodeDevice[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState("");
  const [ok, setOk] = useState("");
  const [busy, setBusy] = useState("");
  const [confirm, setConfirm] = useState<{ kind: ActionKind; row: NodeDevice } | null>(null);
  const [typed, setTyped] = useState("");
  const na = t("nodes.na");

  async function load() {
    setLoading(true);
    try {
      const j = await nodesApi.list();
      const nextPending = asPublic(j.pending);
      const nextPaired = asPublic(j.paired);
      setPending(nextPending);
      setPaired(nextPaired);
      const leak =
        nextPending.some((row) => publicHasSecrets(row)) ||
        nextPaired.some((row) => publicHasSecrets(row)) ||
        (j.pending || []).some((row) => publicHasSecrets(row)) ||
        (j.paired || []).some((row) => publicHasSecrets(row));
      setErr(leak ? t("nodes.leak") : "");
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  function openConfirm(kind: ActionKind, row: NodeDevice) {
    setConfirm({ kind, row });
    setTyped("");
    setOk("");
    setErr("");
  }

  async function submitConfirm() {
    if (!confirm) return;
    if (!nodeConfirmMatch(typed, confirm.row)) {
      setErr(t("nodes.mismatch"));
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
      setErr("");
      setConfirm(null);
      setTyped("");
      await load();
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  const matched = confirm ? nodeConfirmMatch(typed, confirm.row) : false;

  function healthTone(h: string): "neutral" | "accent" | "positive" | "warning" | "critical" {
    if (h === "ok") return "positive";
    if (h === "stale" || h === "pending") return "warning";
    if (h === "expired") return "critical";
    return "neutral";
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
    return rows.map((row) => {
      const rowBusy = busy.endsWith(":" + row.id);
      return (
        <div
          key={row.id}
          style={{
            display: "flex",
            alignItems: "center",
            padding: "11px 16px",
            fontSize: 12.5,
            borderBottom: "1px solid var(--border-soft)",
            gap: 8,
          }}
        >
          <span style={{ flex: 1.6, fontWeight: 600 }}>{nodeLabel(row)}</span>
          <span style={{ flex: 1.1, color: "var(--text-2)" }}>{row.kind || na}</span>
          <span style={{ flex: 1.4, color: "var(--text-3)", fontVariantNumeric: "tabular-nums" }}>
            {kind === "pending" ? formatWhen(row.expires_at, na) : formatWhen(row.last_seen, na)}
          </span>
          <span style={{ flex: 1.1 }}>
            <Badge tone={healthTone(row.health)}>{healthLabel(row.health)}</Badge>
          </span>
          <span style={{ flex: 1.8, display: "flex", gap: 6, justifyContent: "flex-end", flexWrap: "wrap" }}>
            {kind === "pending" ? (
              <>
                <Button variant="quiet" disabled={rowBusy || row.health === "expired"} onClick={() => openConfirm("approve", row)}>
                  {t("nodes.approve")}
                </Button>
                <Button variant="quiet" disabled={rowBusy} onClick={() => openConfirm("deny", row)}>
                  {t("nodes.deny")}
                </Button>
              </>
            ) : (
              <Button variant="quiet" disabled={rowBusy} onClick={() => openConfirm("revoke", row)}>
                {t("nodes.revoke")}
              </Button>
            )}
          </span>
        </div>
      );
    });
  }

  return (
    <div style={{ padding: "14px 22px 40px", display: "flex", flexDirection: "column", gap: 14 }}>
      <SectionHeader
        icon="device"
        title={t("nodes.title")}
        description={t("nodes.desc")}
        actions={
          <Button icon="refresh" iconGesture onClick={() => void load()} disabled={loading || Boolean(busy)}>
            {t("common.refresh")}
          </Button>
        }
      />
      <Card>
        <CardHeader icon="lock" title={t("nodes.how")} />
        <p style={{ margin: 0, padding: "0 16px 14px", fontSize: 12.5, color: "var(--text-3)", maxWidth: 720 }}>
          {t("nodes.howBody")}
        </p>
      </Card>
      {loading ? <StatusLine kind="loading" /> : null}
      {err ? <StatusLine kind="error">{err}</StatusLine> : null}
      {ok && !err ? (
        <p role="status" style={{ margin: 0, fontSize: 12.5, color: "var(--green)" }}>
          {ok}
        </p>
      ) : null}
      {confirm ? (
        <Card>
          <CardHeader icon="lock" title={confirmTitle(confirm.kind)} />
          <div style={{ padding: "0 16px 16px", display: "flex", flexDirection: "column", gap: 10 }}>
            <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-2)" }}>{confirmPreview(confirm.kind, confirm.row)}</p>
            <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("nodes.confirmHint")}</p>
            <input
              className="z-field"
              value={typed}
              onChange={(e) => setTyped(e.target.value)}
              placeholder={t("nodes.confirmPlaceholder")}
              aria-label={t("nodes.confirmPlaceholder")}
              autoComplete="off"
              spellCheck={false}
            />
            <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
              <Button variant="accent" disabled={!matched || Boolean(busy)} onClick={() => void submitConfirm()}>
                {confirmAction(confirm.kind)}
              </Button>
              <Button
                variant="quiet"
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
      <Card>
        <CardHeader icon="hourglass" title={t("nodes.pending")} meta={t("nodes.pending.meta", { n: pending.length })} />
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
            <span style={{ flex: 1.6 }}>{t("nodes.col.device")}</span>
            <span style={{ flex: 1.1 }}>{t("nodes.col.kind")}</span>
            <span style={{ flex: 1.4 }}>{t("nodes.col.expires")}</span>
            <span style={{ flex: 1.1 }}>{t("nodes.col.health")}</span>
            <span style={{ flex: 1.8 }} />
          </div>
          {!loading && pending.length === 0 ? <EmptyState>{t("nodes.pending.empty")}</EmptyState> : null}
          {renderRows(pending, "pending")}
        </TableScroll>
      </Card>
      <Card>
        <CardHeader icon="device" title={t("nodes.paired")} meta={t("nodes.paired.meta", { n: paired.length })} />
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
            <span style={{ flex: 1.6 }}>{t("nodes.col.device")}</span>
            <span style={{ flex: 1.1 }}>{t("nodes.col.kind")}</span>
            <span style={{ flex: 1.4 }}>{t("nodes.col.seen")}</span>
            <span style={{ flex: 1.1 }}>{t("nodes.col.health")}</span>
            <span style={{ flex: 1.8 }} />
          </div>
          {!loading && paired.length === 0 ? <EmptyState>{t("nodes.paired.empty")}</EmptyState> : null}
          {renderRows(paired, "paired")}
        </TableScroll>
      </Card>
    </div>
  );
}
