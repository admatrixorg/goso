import { useEffect, useState } from "react";
import { pendingApi, type PendingGroup } from "../api/pending";
import {
  agentLabel,
  asPublic,
  formatAge,
  groupLabel,
  pendingConfirmMatch,
  previewLine,
  publicHasSecrets,
} from "../api/pending-ops";
import { useI18n } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { SectionHeader } from "../ui/SectionHeader";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

type ConfirmKind = "compact" | "clear";

export function PendingPage() {
  const { t } = useI18n();
  const [groups, setGroups] = useState<PendingGroup[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState("");
  const [ok, setOk] = useState("");
  const [busy, setBusy] = useState("");
  const [confirm, setConfirm] = useState<{ kind: ConfirmKind; group: PendingGroup } | null>(null);
  const [typed, setTyped] = useState("");

  const na = t("pending.na");

  async function load() {
    setLoading(true);
    try {
      const j = await pendingApi.list();
      const rows = asPublic(j.groups);
      setGroups(rows);
      if (rows.some((g) => publicHasSecrets(g)) || (j.groups || []).some((g) => publicHasSecrets(g))) {
        setErr(t("pending.leak"));
      } else {
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

  function openConfirm(kind: ConfirmKind, group: PendingGroup) {
    setConfirm({ kind, group });
    setTyped("");
    setOk("");
  }

  async function submitConfirm() {
    if (!confirm) return;
    const g = confirm.group;
    const name = typed.trim();
    if (!pendingConfirmMatch(name, g)) {
      setErr(t("pending.mismatch"));
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

  const empty = !loading && !err && groups.length === 0;
  const inProgress = groups.some((g) => g.compacting) || busy.startsWith("compact:");
  const matched = confirm ? pendingConfirmMatch(typed, confirm.group) : false;

  return (
    <div style={{ padding: "14px 22px 40px", display: "flex", flexDirection: "column", gap: 14 }}>
      <SectionHeader
        icon="hourglass"
        title={t("pending.title")}
        description={t("pending.desc")}
        actions={
          <Button icon="refresh" iconGesture onClick={() => void load()} disabled={loading || Boolean(busy)}>
            {t("common.refresh")}
          </Button>
        }
      />
      <Card>
        <CardHeader icon="inbox" title={t("pending.how")} />
        <p style={{ margin: 0, padding: "0 16px 14px", fontSize: 12.5, color: "var(--text-3)", maxWidth: 720 }}>
          {t("pending.howBody")}
        </p>
      </Card>
      {loading ? <StatusLine kind="loading" /> : null}
      {err ? <StatusLine kind="error">{err}</StatusLine> : null}
      {inProgress && !err ? (
        <p role="status" style={{ margin: 0, fontSize: 12.5, color: "var(--text-2)" }}>
          {t("pending.compacting")}
        </p>
      ) : null}
      {ok && !err ? (
        <p role="status" style={{ margin: 0, fontSize: 12.5, color: "var(--green)" }}>
          {ok}
        </p>
      ) : null}
      {confirm ? (
        <Card>
          <CardHeader icon="lock" title={t("pending.confirmTitle")} />
          <div style={{ padding: "0 16px 16px", display: "flex", flexDirection: "column", gap: 10 }}>
            <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-2)" }}>
              {t("pending.confirmPreview", { preview: previewLine(confirm.group, na) })}
            </p>
            <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("pending.confirmHint")}</p>
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
                variant="accent"
                disabled={!matched || Boolean(busy)}
                onClick={() => void submitConfirm()}
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
        <CardHeader icon="list" title={t("pending.list")} meta={t("pending.meta", { n: groups.length })} />
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
          {empty ? <EmptyState>{t("pending.empty")}</EmptyState> : null}
          {groups.map((g) => {
            const rowBusy = busy.endsWith(g.id) || Boolean(g.compacting);
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
                  <Button variant="quiet" disabled={rowBusy} onClick={() => openConfirm("compact", g)}>
                    {t("pending.compact")}
                  </Button>
                  <Button variant="quiet" disabled={rowBusy} onClick={() => openConfirm("clear", g)}>
                    {t("pending.clear")}
                  </Button>
                </span>
              </div>
            );
          })}
        </TableScroll>
      </Card>
    </div>
  );
}
