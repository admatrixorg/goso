import { useEffect, useMemo, useState } from "react";
import { contactsApi, type Contact } from "../api/contacts";
import {
  PAGE_SIZE,
  asPublic,
  channelIdsLine,
  filterContacts,
  identLabel,
  lastSourceId,
  mergeConfirmMatch,
  pageOf,
  publicHasSecrets,
  undoConfirmMatch,
  uniqueChannels,
} from "../api/contacts-ops";
import { useI18n } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { SectionHeader } from "../ui/SectionHeader";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

type ConfirmKind = "merge" | "undo";

export function ContactsPage() {
  const { t } = useI18n();
  const [rows, setRows] = useState<Contact[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState("");
  const [ok, setOk] = useState("");
  const [busy, setBusy] = useState("");
  const [q, setQ] = useState("");
  const [channel, setChannel] = useState("");
  const [kind, setKind] = useState("");
  const [offset, setOffset] = useState(0);
  const [selected, setSelected] = useState<string[]>([]);
  const [detailId, setDetailId] = useState("");
  const [confirm, setConfirm] = useState<{ kind: ConfirmKind; target: Contact; source?: Contact } | null>(null);
  const [typed, setTyped] = useState("");

  async function load() {
    setLoading(true);
    try {
      const j = await contactsApi.list({ q, channel, kind });
      const next = asPublic(j.contacts);
      setRows(next);
      if (next.some((row) => publicHasSecrets(row)) || (j.contacts || []).some((row) => publicHasSecrets(row))) {
        setErr(t("contacts.leak"));
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

  const filtered = useMemo(() => filterContacts(rows, q, channel, kind), [rows, q, channel, kind]);
  const page = useMemo(() => pageOf(filtered, offset), [filtered, offset]);
  const channels = useMemo(() => uniqueChannels(rows), [rows]);
  const detail = rows.find((row) => row.id === detailId) || null;
  const empty = !loading && !err && filtered.length === 0;
  const last = Math.max(0, offset + page.length);
  const matched = confirm
    ? confirm.kind === "merge" && confirm.source
      ? mergeConfirmMatch(typed, confirm.target, confirm.source)
      : undoConfirmMatch(typed, confirm.target, lastSourceId(confirm.target))
    : false;

  function permLabel(p: string | undefined): string {
    if (p === "group") return t("contacts.perm.group");
    return t("contacts.perm.direct");
  }

  function kindLabel(k: string): string {
    return k === "group" ? t("contacts.kind.group") : t("contacts.kind.user");
  }

  function toggleSelect(id: string) {
    setSelected((cur) => (cur.includes(id) ? cur.filter((x) => x !== id) : [...cur, id]));
  }

  function openMerge() {
    if (selected.length !== 2) {
      setErr(t("contacts.needTwo"));
      return;
    }
    const [a, b] = selected;
    const targetId = selected.includes(detailId) ? detailId : a;
    const sourceId = targetId === a ? b : a;
    const target = rows.find((row) => row.id === targetId);
    const source = rows.find((row) => row.id === sourceId);
    if (!target || !source) {
      setErr(t("contacts.needTwo"));
      return;
    }
    setConfirm({ kind: "merge", target, source });
    setTyped("");
    setOk("");
    setErr("");
  }

  function openUndo(row: Contact) {
    setConfirm({ kind: "undo", target: row });
    setTyped("");
    setOk("");
    setErr("");
  }

  async function submitConfirm() {
    if (!confirm) return;
    if (confirm.kind === "merge") {
      if (!confirm.source || !mergeConfirmMatch(typed, confirm.target, confirm.source)) {
        setErr(t("contacts.mismatch"));
        return;
      }
      setBusy(`merge:${confirm.target.id}`);
      try {
        await contactsApi.merge(confirm.target.id, confirm.source.id, typed.trim());
        setOk(t("contacts.mergeOk"));
        setErr("");
        setConfirm(null);
        setTyped("");
        setSelected([]);
        await load();
      } catch (e) {
        setErr(formatPublicError(e));
      } finally {
        setBusy("");
      }
      return;
    }
    if (!undoConfirmMatch(typed, confirm.target, lastSourceId(confirm.target))) {
      setErr(t("contacts.mismatch"));
      return;
    }
    setBusy(`undo:${confirm.target.id}`);
    try {
      await contactsApi.undo(confirm.target.id, typed.trim());
      setOk(t("contacts.undoOk"));
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

  return (
    <div style={{ padding: "14px 22px 40px", display: "flex", flexDirection: "column", gap: 14 }}>
      <SectionHeader
        icon="user"
        title={t("contacts.title")}
        description={t("contacts.desc")}
        actions={
          <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
            <Button icon="refresh" iconGesture onClick={() => void load()} disabled={loading || Boolean(busy)}>
              {t("common.refresh")}
            </Button>
            <Button variant="accent" disabled={selected.length !== 2 || Boolean(busy)} onClick={openMerge}>
              {t("contacts.merge")}
            </Button>
          </div>
        }
      />
      <Card>
        <CardHeader icon="lock" title={t("contacts.how")} />
        <p style={{ margin: 0, padding: "0 16px 14px", fontSize: 12.5, color: "var(--text-3)", maxWidth: 720 }}>
          {t("contacts.howBody")}
        </p>
      </Card>
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "center" }}>
        <input
          className="z-field"
          value={q}
          onChange={(e) => {
            setQ(e.target.value);
            setOffset(0);
          }}
          placeholder={t("contacts.search")}
          aria-label={t("contacts.search")}
          style={{ minWidth: 220, flex: 1 }}
        />
        <label style={{ display: "flex", gap: 6, alignItems: "center", fontSize: 12, color: "var(--text-3)" }}>
          {t("contacts.filter.channel")}
          <select
            className="z-field"
            value={channel}
            onChange={(e) => {
              setChannel(e.target.value);
              setOffset(0);
            }}
            aria-label={t("contacts.filter.channel")}
          >
            <option value="">{t("contacts.filter.all")}</option>
            {channels.map((ch) => (
              <option key={ch} value={ch}>
                {ch}
              </option>
            ))}
          </select>
        </label>
        <label style={{ display: "flex", gap: 6, alignItems: "center", fontSize: 12, color: "var(--text-3)" }}>
          {t("contacts.filter.kind")}
          <select
            className="z-field"
            value={kind}
            onChange={(e) => {
              setKind(e.target.value);
              setOffset(0);
            }}
            aria-label={t("contacts.filter.kind")}
          >
            <option value="">{t("contacts.filter.all")}</option>
            <option value="user">{t("contacts.kind.user")}</option>
            <option value="group">{t("contacts.kind.group")}</option>
          </select>
        </label>
      </div>
      {loading ? <StatusLine kind="loading" /> : null}
      {err ? <StatusLine kind="error">{err}</StatusLine> : null}
      {ok && !err ? (
        <p role="status" style={{ margin: 0, fontSize: 12.5, color: "var(--green)" }}>
          {ok}
        </p>
      ) : null}
      {confirm ? (
        <Card>
          <CardHeader icon="lock" title={confirm.kind === "merge" ? t("contacts.confirmTitle") : t("contacts.confirmUndoTitle")} />
          <div style={{ padding: "0 16px 16px", display: "flex", flexDirection: "column", gap: 10 }}>
            <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-2)" }}>
              {confirm.kind === "merge" && confirm.source
                ? t("contacts.confirmPreview", { source: confirm.source.display || confirm.source.id, target: confirm.target.display || confirm.target.id })
                : t("contacts.confirmUndoPreview", { target: confirm.target.display || confirm.target.id })}
            </p>
            <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>
              {confirm.kind === "merge" ? t("contacts.confirmHint") : t("contacts.confirmUndoHint")}
            </p>
            <input
              className="z-field"
              value={typed}
              onChange={(e) => setTyped(e.target.value)}
              placeholder={t("contacts.confirmPlaceholder")}
              aria-label={t("contacts.confirmPlaceholder")}
              autoComplete="off"
              spellCheck={false}
            />
            <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
              <Button variant="accent" disabled={!matched || Boolean(busy)} onClick={() => void submitConfirm()}>
                {confirm.kind === "merge" ? t("contacts.confirmMerge") : t("contacts.confirmUndo")}
              </Button>
              <Button
                variant="quiet"
                disabled={Boolean(busy)}
                onClick={() => {
                  setConfirm(null);
                  setTyped("");
                }}
              >
                {t("contacts.cancel")}
              </Button>
            </div>
          </div>
        </Card>
      ) : null}
      <Card>
        <CardHeader icon="list" title={t("contacts.list")} meta={t("contacts.meta", { n: filtered.length })} />
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
            <span style={{ width: 28 }} />
            <span style={{ flex: 1.8 }}>{t("contacts.col.identity")}</span>
            <span style={{ flex: 0.8 }}>{t("contacts.col.kind")}</span>
            <span style={{ flex: 1.8 }}>{t("contacts.col.channels")}</span>
            <span style={{ flex: 1.2 }}>{t("contacts.col.seen")}</span>
            <span style={{ flex: 1.4 }}>{t("contacts.col.permission")}</span>
          </div>
          {empty ? <EmptyState>{t("contacts.empty")}</EmptyState> : null}
          {page.map((row) => {
            const on = selected.includes(row.id);
            const open = detailId === row.id;
            return (
              <div
                key={row.id}
                role="button"
                tabIndex={0}
                onClick={() => setDetailId(row.id)}
                onKeyDown={(e) => {
                  if (e.key === "Enter" || e.key === " ") {
                    e.preventDefault();
                    setDetailId(row.id);
                  }
                }}
                style={{
                  display: "flex",
                  alignItems: "center",
                  padding: "11px 16px",
                  fontSize: 12.5,
                  borderBottom: "1px solid var(--border-soft)",
                  gap: 8,
                  background: open ? "var(--accent-soft)" : "transparent",
                  cursor: "pointer",
                }}
              >
                <span style={{ width: 28 }} onClick={(e) => e.stopPropagation()}>
                  <input type="checkbox" checked={on} onChange={() => toggleSelect(row.id)} aria-label={row.display || row.id} />
                </span>
                <span style={{ flex: 1.8, fontWeight: 600 }}>{row.display || row.id}</span>
                <span style={{ flex: 0.8 }}>
                  <Badge tone={row.kind === "group" ? "warning" : "neutral"}>{kindLabel(row.kind)}</Badge>
                </span>
                <span style={{ flex: 1.8, color: "var(--text-2)" }}>{channelIdsLine(row)}</span>
                <span style={{ flex: 1.2, color: "var(--text-3)", fontVariantNumeric: "tabular-nums" }}>{row.last_seen || t("contacts.na")}</span>
                <span style={{ flex: 1.4, color: "var(--text-2)" }}>{permLabel(row.permission)}</span>
              </div>
            );
          })}
        </TableScroll>
        {filtered.length > PAGE_SIZE ? (
          <div style={{ display: "flex", gap: 8, alignItems: "center", padding: "10px 16px", fontSize: 12.5 }}>
            <Button variant="quiet" disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}>
              {t("contacts.prev")}
            </Button>
            <span style={{ color: "var(--text-3)" }}>{t("contacts.page", { from: filtered.length ? offset + 1 : 0, to: last, n: filtered.length })}</span>
            <Button variant="quiet" disabled={last >= filtered.length} onClick={() => setOffset(offset + PAGE_SIZE)}>
              {t("contacts.next")}
            </Button>
          </div>
        ) : null}
      </Card>
      {detail ? (
        <Card>
          <CardHeader icon="user" title={t("contacts.detail")} meta={detail.id} />
          <div style={{ padding: "0 16px 16px", display: "flex", flexDirection: "column", gap: 10, fontSize: 12.5 }}>
            <div>
              <div style={{ fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)" }}>{t("contacts.canonical")}</div>
              <div style={{ fontWeight: 600 }}>{detail.display || detail.id}</div>
              <div style={{ color: "var(--text-3)" }}>
                {kindLabel(detail.kind)} · {detail.channel}:{detail.dest}
              </div>
            </div>
            <div>
              <div style={{ fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)" }}>{t("contacts.ids")}</div>
              {(detail.identifiers || []).map((id) => (
                <div key={identLabel(id)} style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
                  <span>{identLabel(id)}</span>
                  <Badge tone="neutral">{kindLabel(id.kind)}</Badge>
                  <span style={{ color: "var(--text-3)" }}>{permLabel(id.permission)}</span>
                </div>
              ))}
            </div>
            <div>
              <div style={{ fontSize: 10, fontWeight: 600, letterSpacing: ".4px", color: "var(--text-3)" }}>{t("contacts.provenance")}</div>
              <div style={{ color: "var(--text-2)" }}>
                {detail.first_seen || t("contacts.na")} → {detail.last_seen || t("contacts.na")}
              </div>
              {detail.merged_from && detail.merged_from.length ? (
                <div>{t("contacts.mergedFrom", { ids: detail.merged_from.join(", ") })}</div>
              ) : null}
            </div>
            {detail.can_undo ? (
              <div>
                <Button variant="quiet" disabled={Boolean(busy)} onClick={() => openUndo(detail)}>
                  {t("contacts.undo")}
                </Button>
              </div>
            ) : null}
          </div>
        </Card>
      ) : null}
    </div>
  );
}
