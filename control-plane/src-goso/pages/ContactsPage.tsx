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
  mergePair,
  publicHasSecrets,
  swapMergePair,
  undoConfirmMatch,
  uniqueChannels,
} from "../api/contacts-ops";
import {
  classifyPageState,
  clampPageOffset,
  formatStaleAt,
  inventoryBlocksMutation,
  isFilteredEmpty,
  listMetaCount,
  pageSlice,
} from "../api/page-state";
import { useI18n } from "../i18n";
import { Button } from "../ui/Button";
import { Card, CardHeader } from "../ui/Card";
import { DataTable, THead, Th, Td, Tr } from "../ui/DataTable";
import { EmptyState } from "../ui/EmptyState";
import { PageChrome } from "../ui/PageChrome";
import { PageStatus } from "../ui/PageStatus";
import { SearchInput } from "../ui/SearchInput";
import { StatusBadge } from "../ui/StatusBadge";
import { StatusLine, formatPublicError } from "../ui/StatusLine";
import { fieldClass, labelClass, selectClass } from "../ui/field";
import { FoldedMarketingLink } from "./FeaturePages";

type ConfirmKind = "merge" | "undo";

export function ContactsPage() {
  const { t, locale } = useI18n();
  const [rows, setRows] = useState<Contact[]>([]);
  const [loading, setLoading] = useState(true);
  const [loaded, setLoaded] = useState(false);
  const [loadedAt, setLoadedAt] = useState<string | null>(null);
  const [err, setErr] = useState<unknown>(null);
  const [actionErr, setActionErr] = useState("");
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

  const state = classifyPageState({
    loading,
    loaded,
    error: err,
    itemCount: rows.length,
    keepStale: loaded && rows.length > 0,
  });
  const blocked = inventoryBlocksMutation(state.kind);

  async function load() {
    setLoading(true);
    try {
      const j = await contactsApi.list({ q, channel, kind });
      const next = asPublic(j.contacts);
      setRows(next);
      setLoaded(true);
      setLoadedAt(new Date().toISOString());
      if (next.some((row) => publicHasSecrets(row)) || (j.contacts || []).some((row) => publicHasSecrets(row))) {
        setActionErr(t("contacts.leak"));
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

  const filtered = useMemo(() => filterContacts(rows, q, channel, kind), [rows, q, channel, kind]);
  const safeOffset = clampPageOffset(filtered.length, offset, PAGE_SIZE);
  const page = useMemo(() => pageSlice(filtered, safeOffset, PAGE_SIZE), [filtered, safeOffset]);
  const channels = useMemo(() => uniqueChannels(rows), [rows]);
  const detail = rows.find((row) => row.id === detailId) || null;
  const filteredEmpty = isFilteredEmpty(state, rows.length, filtered.length);
  const last = Math.max(0, safeOffset + page.length);
  const metaN = listMetaCount(state.kind, filtered.length);
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
    if (blocked) return;
    const pair = mergePair(selected, detailId, rows);
    if (!pair) {
      setActionErr(t("contacts.needTwo"));
      return;
    }
    setConfirm({ kind: "merge", target: pair.target, source: pair.source });
    setTyped("");
    setOk("");
    setActionErr("");
  }

  function openUndo(row: Contact) {
    if (blocked) return;
    setConfirm({ kind: "undo", target: row });
    setTyped("");
    setOk("");
    setActionErr("");
  }

  function recoverAfterWrite(nextDetail?: string) {
    setSelected([]);
    setOffset(0);
    if (nextDetail) setDetailId(nextDetail);
  }

  async function submitConfirm() {
    if (!confirm || blocked) return;
    if (confirm.kind === "merge") {
      if (!confirm.source || !mergeConfirmMatch(typed, confirm.target, confirm.source)) {
        setActionErr(t("contacts.mismatch"));
        return;
      }
      setBusy(`merge:${confirm.target.id}`);
      try {
        await contactsApi.merge(confirm.target.id, confirm.source.id, typed.trim());
        setOk(t("contacts.mergeOk"));
        setActionErr("");
        const keep = confirm.target.id;
        setConfirm(null);
        setTyped("");
        recoverAfterWrite(keep);
        await load();
      } catch (e) {
        setActionErr(formatPublicError(e));
      } finally {
        setBusy("");
      }
      return;
    }
    if (!undoConfirmMatch(typed, confirm.target, lastSourceId(confirm.target))) {
      setActionErr(t("contacts.mismatch"));
      return;
    }
    setBusy(`undo:${confirm.target.id}`);
    try {
      await contactsApi.undo(confirm.target.id, typed.trim());
      setOk(t("contacts.undoOk"));
      setActionErr("");
      const keep = confirm.target.id;
      setConfirm(null);
      setTyped("");
      recoverAfterWrite(keep);
      await load();
    } catch (e) {
      setActionErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  return (
    <PageChrome
      icon="contact"
      title={t("contacts.title")}
      description={t("contacts.desc")}
      primary={
        <Button variant="default" disabled={blocked || selected.length !== 2 || Boolean(busy)} onClick={openMerge}>
          {t("contacts.merge")}
        </Button>
      }
      refresh={
        <Button variant="outline" icon="refresh" iconGesture onClick={() => void load()} disabled={loading || Boolean(busy)}>
          {t("common.refresh")}
        </Button>
      }
      filters={
        <>
          <SearchInput
            value={q}
            onChange={(v) => {
              setQ(v);
              setOffset(0);
            }}
            placeholder={t("contacts.search")}
            aria-label={t("contacts.search")}
          />
          <label className={`${labelClass} flex items-center gap-2`}>
            {t("contacts.filter.channel")}
            <select
              className={selectClass}
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
          <label className={`${labelClass} flex items-center gap-2`}>
            {t("contacts.filter.kind")}
            <select
              className={selectClass}
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
        </>
      }
    >
      <FoldedMarketingLink />
      <Card>
        <CardHeader icon="lock" title={t("contacts.how")} />
        <p className="max-w-xl p-4 text-sm text-muted-foreground">{t("contacts.howBody")}</p>
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
          <CardHeader icon="lock" title={confirm.kind === "merge" ? t("contacts.confirmTitle") : t("contacts.confirmUndoTitle")} />
          <div className="flex flex-col gap-3 p-4">
            {confirm.kind === "merge" && confirm.source ? (
              <>
                <p className="text-sm">
                  {t("contacts.target")}: <b>{confirm.target.display || confirm.target.id}</b>
                  {" · "}
                  {t("contacts.source")}: <b>{confirm.source.display || confirm.source.id}</b>
                </p>
                <p className="text-sm">
                  {t("contacts.confirmPreview", { source: confirm.source.display || confirm.source.id, target: confirm.target.display || confirm.target.id })}
                </p>
                <p className="text-sm text-muted-foreground">{t("contacts.loss")}</p>
                <Button
                  variant="outline"
                  disabled={Boolean(busy)}
                  onClick={() => setConfirm((cur) => (cur?.source ? { ...cur, ...swapMergePair({ target: cur.target, source: cur.source }) } : cur))}
                >
                  {t("contacts.swap")}
                </Button>
              </>
            ) : (
              <p className="text-sm">{t("contacts.confirmUndoPreview", { target: confirm.target.display || confirm.target.id })}</p>
            )}
            <p className="text-sm text-muted-foreground">
              {confirm.kind === "merge" ? t("contacts.confirmHint") : t("contacts.confirmUndoHint")}
            </p>
            <input
              className={fieldClass}
              value={typed}
              onChange={(e) => setTyped(e.target.value)}
              placeholder={t("contacts.confirmPlaceholder")}
              aria-label={t("contacts.confirmPlaceholder")}
              autoComplete="off"
              spellCheck={false}
            />
            <div className="flex flex-wrap gap-2">
              <Button variant="default" disabled={!matched || Boolean(busy)} onClick={() => void submitConfirm()}>
                {confirm.kind === "merge" ? t("contacts.confirmMerge") : t("contacts.confirmUndo")}
              </Button>
              <Button
                variant="outline"
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
      {state.showEmpty ? <EmptyState icon="contact" title={t("contacts.empty")} /> : null}
      {filteredEmpty ? <EmptyState icon="search" title={t("contacts.emptyFilter")} /> : null}
      {state.showItems ? (
        <p className="text-xs text-muted-foreground">{metaN == null ? "—" : t("contacts.meta", { n: metaN })}</p>
      ) : null}
      {state.showItems ? (
        <DataTable minWidth="720px">
          <THead>
            <Th className="w-10" />
            <Th>{t("contacts.col.identity")}</Th>
            <Th>{t("contacts.col.kind")}</Th>
            <Th>{t("contacts.col.channels")}</Th>
            <Th>{t("contacts.col.seen")}</Th>
            <Th>{t("contacts.col.permission")}</Th>
          </THead>
          <tbody>
            {page.map((row) => {
              const on = selected.includes(row.id);
              return (
                <Tr key={row.id} selected={detailId === row.id} onClick={() => setDetailId(row.id)}>
                  <Td onClick={(e) => e.stopPropagation()}>
                    <input type="checkbox" checked={on} onChange={() => toggleSelect(row.id)} aria-label={row.display || row.id} disabled={blocked} />
                  </Td>
                  <Td className="font-semibold">{row.display || row.id}</Td>
                  <Td>
                    <StatusBadge status={row.kind === "group" ? "warning" : "default"} label={kindLabel(row.kind)} />
                  </Td>
                  <Td className="text-muted-foreground">{channelIdsLine(row)}</Td>
                  <Td className="tabular-nums text-muted-foreground">{row.last_seen || t("contacts.na")}</Td>
                  <Td className="text-muted-foreground">{permLabel(row.permission)}</Td>
                </Tr>
              );
            })}
          </tbody>
        </DataTable>
      ) : null}
      {state.showItems && filtered.length > PAGE_SIZE ? (
        <div className="flex flex-wrap items-center gap-2 text-sm">
          <Button variant="outline" size="sm" disabled={safeOffset === 0} onClick={() => setOffset(Math.max(0, safeOffset - PAGE_SIZE))}>
            {t("contacts.prev")}
          </Button>
          <span className="text-muted-foreground">{t("contacts.page", { from: filtered.length ? safeOffset + 1 : 0, to: last, n: filtered.length })}</span>
          <Button variant="outline" size="sm" disabled={last >= filtered.length} onClick={() => setOffset(safeOffset + PAGE_SIZE)}>
            {t("contacts.next")}
          </Button>
        </div>
      ) : null}
      {detail && state.showItems ? (
        <Card>
          <CardHeader icon="user" title={t("contacts.detail")} meta={detail.id} />
          <div className="flex flex-col gap-3 p-4 text-sm">
            <div>
              <div className={labelClass}>{t("contacts.canonical")}</div>
              <div className="font-semibold">{detail.display || detail.id}</div>
              <div className="text-muted-foreground">
                {kindLabel(detail.kind)} · {detail.channel}:{detail.dest}
              </div>
            </div>
            <div>
              <div className={labelClass}>{t("contacts.ids")}</div>
              {(detail.identifiers || []).map((id) => (
                <div key={identLabel(id)} className="flex flex-wrap items-center gap-2">
                  <span>{identLabel(id)}</span>
                  <StatusBadge status={id.kind === "group" ? "warning" : "default"} label={kindLabel(id.kind)} />
                  <span className="text-muted-foreground">{permLabel(id.permission)}</span>
                </div>
              ))}
            </div>
            <div>
              <div className={labelClass}>{t("contacts.provenance")}</div>
              <div className="text-muted-foreground">
                {detail.first_seen || t("contacts.na")} → {detail.last_seen || t("contacts.na")}
              </div>
              {detail.merged_from && detail.merged_from.length ? <div>{t("contacts.mergedFrom", { ids: detail.merged_from.join(", ") })}</div> : null}
            </div>
            {detail.can_undo && !blocked ? (
              <div>
                <Button variant="ghost" size="sm" disabled={Boolean(busy)} onClick={() => openUndo(detail)}>
                  {t("contacts.undo")}
                </Button>
              </div>
            ) : null}
          </div>
        </Card>
      ) : null}
    </PageChrome>
  );
}
