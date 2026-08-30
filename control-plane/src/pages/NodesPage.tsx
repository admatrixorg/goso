import { useEffect, useState } from "react";
import { nodesApi, type NodeDevice } from "../api/nodes";
import { asPublic, formatWhen, nodeConfirmMatch, nodeInventoryCount, nodeLabel, publicHasSecrets } from "../api/nodes-ops";
import { classifyPageState, formatStaleAt, inventoryBlocksMutation, listMetaCount } from "../api/page-state";
import { useI18n } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { PageChrome } from "../ui/PageChrome";
import { PageStatus } from "../ui/PageStatus";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

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
    if (!state.showItems) return null;
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
                <Button variant="quiet" disabled={blocked || rowBusy || row.health === "expired"} onClick={() => openConfirm("approve", row)}>
                  {t("nodes.approve")}
                </Button>
                <Button variant="quiet" disabled={blocked || rowBusy} onClick={() => openConfirm("deny", row)}>
                  {t("nodes.deny")}
                </Button>
              </>
            ) : (
              <Button variant="quiet" disabled={blocked || rowBusy} onClick={() => openConfirm("revoke", row)}>
                {t("nodes.revoke")}
              </Button>
            )}
          </span>
        </div>
      );
    });
  }

  return (
    <PageChrome
      icon="device"
      title={t("nodes.title")}
      description={t("nodes.desc")}
      primary={
        <Button icon="refresh" iconGesture variant="primary" onClick={() => void load()} disabled={loading || Boolean(busy)}>
          {t("common.refresh")}
        </Button>
      }
    >
      <Card>
        <CardHeader icon="lock" title={t("nodes.how")} />
        <p style={{ margin: 0, padding: "0 16px 14px", fontSize: 12.5, color: "var(--text-3)", maxWidth: 720 }}>
          {t("nodes.howBody")}
        </p>
      </Card>
      <PageStatus kind={state.kind} errorText={err ? formatPublicError(err) : ""} staleAt={formatStaleAt(loadedAt, locale)} onReload={() => void load()} />
      {actionErr ? <StatusLine kind="error">{actionErr}</StatusLine> : null}
      {ok && !actionErr ? (
        <p role="status" style={{ margin: 0, fontSize: 12.5, color: "var(--green)" }}>
          {ok}
        </p>
      ) : null}
      {confirm && !blocked ? (
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
              <Button
                variant={confirm.kind === "approve" ? "accent" : "primary"}
                disabled={!matched || Boolean(busy)}
                onClick={() => void submitConfirm()}
                style={confirm.kind !== "approve" ? { background: "var(--red)", borderColor: "transparent" } : undefined}
              >
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
        <CardHeader icon="hourglass" title={t("nodes.pending")} meta={pendingMeta == null ? "—" : t("nodes.pending.meta", { n: pendingMeta })} />
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
          {state.showEmpty || (state.showItems && pending.length === 0) ? <EmptyState>{t("nodes.pending.empty")}</EmptyState> : null}
          {renderRows(pending, "pending")}
        </TableScroll>
      </Card>
      <Card>
        <CardHeader icon="device" title={t("nodes.paired")} meta={pairedMeta == null ? "—" : t("nodes.paired.meta", { n: pairedMeta })} />
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
          {state.showEmpty || (state.showItems && paired.length === 0) ? <EmptyState>{t("nodes.paired.empty")}</EmptyState> : null}
          {renderRows(paired, "paired")}
        </TableScroll>
      </Card>
    </PageChrome>
  );
}
