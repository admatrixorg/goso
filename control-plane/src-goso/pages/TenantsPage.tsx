import { useEffect, useMemo, useState } from "react";
import { tenantsApi, type Tenant, type TenantList, type TenantMember } from "../api/tenants";
import {
  ROLES,
  filterTenants,
  formatWhen,
  memberConfirmMatch,
  publicHasSecrets,
  tenantConfirmMatch,
  tenantLabel,
} from "../api/tenants-ops";
import { classifyPageState, formatStaleAt, inventoryBlocksMutation, listMetaCount } from "../api/page-state";
import { useI18n } from "../i18n";
import { Button } from "../ui/Button";
import { Card, CardHeader } from "../ui/Card";
import { DataTable, THead, Th, Td, Tr } from "../ui/DataTable";
import { EmptyState } from "../ui/EmptyState";
import { PageChrome } from "../ui/PageChrome";
import { PageStatus } from "../ui/PageStatus";
import { SearchInput } from "../ui/SearchInput";
import { StatusBadge, type Status } from "../ui/StatusBadge";
import { StatusLine, formatPublicError } from "../ui/StatusLine";
import { fieldClass, labelClass, selectClass } from "../ui/field";

type ConfirmKind = "deactivate" | "remove";

const emptyList = (): TenantList => ({ tenants: [] });

export function TenantsPage() {
  const { t, locale } = useI18n();
  const [list, setList] = useState<TenantList>(emptyList);
  const [detail, setDetail] = useState<Tenant | null>(null);
  const [selected, setSelected] = useState("");
  const [q, setQ] = useState("");
  const [loading, setLoading] = useState(true);
  const [loaded, setLoaded] = useState(false);
  const [loadedAt, setLoadedAt] = useState<string | null>(null);
  const [err, setErr] = useState<unknown>(null);
  const [detailErr, setDetailErr] = useState("");
  const [ok, setOk] = useState("");
  const [busy, setBusy] = useState("");
  const [slug, setSlug] = useState("");
  const [name, setName] = useState("");
  const [subject, setSubject] = useState("");
  const [role, setRole] = useState("member");
  const [confirm, setConfirm] = useState<{ kind: ConfirmKind; tenant: Tenant; member?: TenantMember } | null>(null);
  const [typed, setTyped] = useState("");
  const na = t("tenants.na");

  const state = classifyPageState({
    loading,
    loaded,
    error: err,
    itemCount: list.tenants.length,
    keepStale: loaded && list.tenants.length > 0,
  });
  const blocked = inventoryBlocksMutation(state.kind);
  const filtersOn = q.trim().length > 0;
  const filtered = useMemo(() => filterTenants(state.showItems ? list.tenants : [], q), [list.tenants, q, state.showItems]);
  const trueEmpty = state.showEmpty && !filtersOn;
  const filterEmpty = state.showEmpty && filtersOn;
  const metaN = listMetaCount(state.kind, list.tenants.length);
  const current = list.current;
  const master = list.master;
  const matched = confirm
    ? confirm.kind === "deactivate"
      ? tenantConfirmMatch(typed, confirm.tenant)
      : confirm.member
        ? memberConfirmMatch(typed, confirm.member)
        : false
    : false;

  async function load(keepId = selected) {
    setLoading(true);
    try {
      const j = await tenantsApi.list(q);
      const leak = (j.tenants || []).some((row) => publicHasSecrets(row));
      setList(j);
      setLoaded(true);
      setLoadedAt(new Date().toISOString());
      setErr(null);
      setDetailErr(leak ? t("tenants.leak") : "");
      const id = keepId && j.tenants.some((r) => r.id === keepId) ? keepId : j.tenants[0]?.id || "";
      setSelected(id);
      if (id) {
        try {
          const d = await tenantsApi.get(id);
          setDetail(d);
          if (publicHasSecrets(d)) setDetailErr(t("tenants.leak"));
        } catch (e) {
          setDetail(null);
          setDetailErr(formatPublicError(e));
        }
      } else {
        setDetail(null);
      }
    } catch (e) {
      setErr(e);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load(selected);
  }, [q]);

  async function createTenant() {
    if (blocked) return;
    setBusy("create");
    setOk("");
    try {
      const row = await tenantsApi.create(slug.trim(), name.trim());
      setSlug("");
      setName("");
      setOk(t("tenants.createOk"));
      setDetailErr("");
      await load(row.id);
    } catch (e) {
      setDetailErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  async function pick(id: string) {
    if (blocked) return;
    setSelected(id);
    setConfirm(null);
    setTyped("");
    setOk("");
    try {
      const d = await tenantsApi.get(id);
      setDetail(d);
      setDetailErr(publicHasSecrets(d) ? t("tenants.leak") : "");
    } catch (e) {
      setDetailErr(formatPublicError(e));
      setDetail(null);
    }
  }

  async function activate() {
    if (!detail || blocked) return;
    setBusy("status");
    try {
      const row = await tenantsApi.setStatus(detail.id, "active");
      setOk(t("tenants.activateOk"));
      setDetailErr("");
      await load(row.id);
    } catch (e) {
      setDetailErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  async function addMember() {
    if (!detail || blocked) return;
    setBusy("member");
    try {
      const row = await tenantsApi.addMember(detail.id, subject.trim(), role);
      setSubject("");
      setRole("member");
      setOk(t("tenants.memberOk"));
      setDetailErr("");
      setDetail(row);
      await load(row.id);
    } catch (e) {
      setDetailErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  async function changeRole(mid: string, next: string) {
    if (!detail || blocked) return;
    setBusy("role:" + mid);
    try {
      const row = await tenantsApi.setMemberRole(detail.id, mid, next);
      setOk(t("tenants.roleOk"));
      setDetailErr("");
      setDetail(row);
    } catch (e) {
      setDetailErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  async function submitConfirm() {
    if (!confirm || blocked) return;
    if (confirm.kind === "deactivate" && !tenantConfirmMatch(typed, confirm.tenant)) {
      setDetailErr(t("tenants.mismatch"));
      return;
    }
    if (confirm.kind === "remove" && confirm.member && !memberConfirmMatch(typed, confirm.member)) {
      setDetailErr(t("tenants.mismatch"));
      return;
    }
    setBusy(confirm.kind);
    try {
      if (confirm.kind === "deactivate") {
        await tenantsApi.setStatus(confirm.tenant.id, "deactivated", typed.trim());
        setOk(t("tenants.deactivateOk"));
      } else if (confirm.member) {
        await tenantsApi.removeMember(confirm.tenant.id, confirm.member.id, typed.trim());
        setOk(t("tenants.removeOk"));
      }
      setDetailErr("");
      setConfirm(null);
      setTyped("");
      await load(confirm.tenant.id);
    } catch (e) {
      setDetailErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  function statusOf(st: string): Status {
    if (st === "active") return "success";
    if (st === "deactivated") return "warning";
    return "default";
  }

  function statusLabel(st: string): string {
    if (st === "active") return t("tenants.status.active");
    if (st === "deactivated") return t("tenants.status.deactivated");
    return st || na;
  }

  function roleLabel(r: string): string {
    if (r === "owner") return t("tenants.role.owner");
    if (r === "admin") return t("tenants.role.admin");
    if (r === "member") return t("tenants.role.member");
    if (r === "viewer") return t("tenants.role.viewer");
    return r;
  }

  const showDetail = Boolean(detail) && (state.showItems || state.kind === "stale");

  return (
    <PageChrome
      icon="building"
      title={t("tenants.title")}
      description={t("tenants.desc")}
      primary={
        <Button
          variant="default"
          icon="plus"
          disabled={blocked || Boolean(busy) || !slug.trim() || !name.trim()}
          onClick={() => void createTenant()}
        >
          {t("tenants.create")}
        </Button>
      }
      refresh={
        <Button variant="outline" icon="refresh" iconGesture onClick={() => void load(selected)} disabled={loading || Boolean(busy)}>
          {t("common.refresh")}
        </Button>
      }
      filters={<SearchInput value={q} onChange={setQ} placeholder={t("tenants.search")} aria-label={t("tenants.search")} />}
    >
      <Card>
        <CardHeader icon="shield" title={t("tenants.context")} />
        <p className="px-4 pb-4 text-sm leading-relaxed">
          {t("tenants.context.body", {
            current: current?.id || "default",
            currentName: current?.name || "default",
            master: master?.id || "default",
            masterName: master?.name || "Master",
            mode: list.multi_tenant ? t("tenants.mode.multi") : t("tenants.mode.single"),
          })}
        </p>
      </Card>
      <PageStatus kind={state.kind} errorText={err ? formatPublicError(err) : ""} staleAt={formatStaleAt(loadedAt, locale)} onReload={() => void load(selected)} />
      {detailErr ? <StatusLine kind="error">{detailErr}</StatusLine> : null}
      {ok && !detailErr ? (
        <p role="status" className="text-sm text-success">
          {ok}
        </p>
      ) : null}
      <Card>
        <CardHeader icon="plus" title={t("tenants.create")} />
        <div className="flex flex-wrap items-end gap-3 p-4">
          <label className={labelClass + " min-w-[140px]"}>
            {t("tenants.slug")}
            <input
              className={fieldClass + " mt-1"}
              value={slug}
              disabled={blocked}
              onChange={(e) => setSlug(e.target.value)}
              placeholder={t("tenants.slug")}
              aria-label={t("tenants.slug")}
              autoComplete="off"
              spellCheck={false}
            />
          </label>
          <label className={labelClass + " min-w-[180px] flex-1"}>
            {t("tenants.name")}
            <input
              className={fieldClass + " mt-1"}
              value={name}
              disabled={blocked}
              onChange={(e) => setName(e.target.value)}
              placeholder={t("tenants.name")}
              aria-label={t("tenants.name")}
              autoComplete="off"
            />
          </label>
        </div>
      </Card>
      {confirm && !blocked ? (
        <Card>
          <CardHeader icon="lock" title={confirm.kind === "deactivate" ? t("tenants.confirmDeactivateTitle") : t("tenants.confirmRemoveTitle")} />
          <div className="flex flex-col gap-3 p-4">
            <p className="text-sm">
              {confirm.kind === "deactivate"
                ? t("tenants.confirmDeactivatePreview", { name: tenantLabel(confirm.tenant) })
                : t("tenants.confirmRemovePreview", { name: confirm.member?.subject || "" })}
            </p>
            <p className="text-sm text-muted-foreground">
              {confirm.kind === "deactivate" ? t("tenants.confirmHint") : t("tenants.confirmMemberHint")}
            </p>
            <input
              className={fieldClass}
              value={typed}
              onChange={(e) => setTyped(e.target.value)}
              placeholder={confirm.kind === "deactivate" ? t("tenants.confirmPlaceholder") : t("tenants.confirmMemberPlaceholder")}
              aria-label={confirm.kind === "deactivate" ? t("tenants.confirmPlaceholder") : t("tenants.confirmMemberPlaceholder")}
              autoComplete="off"
              spellCheck={false}
            />
            <div className="flex flex-wrap gap-2">
              <Button variant="destructive" disabled={!matched || Boolean(busy)} onClick={() => void submitConfirm()}>
                {confirm.kind === "deactivate" ? t("tenants.confirmDeactivate") : t("tenants.confirmRemove")}
              </Button>
              <Button
                variant="outline"
                disabled={Boolean(busy)}
                onClick={() => {
                  setConfirm(null);
                  setTyped("");
                }}
              >
                {t("tenants.cancel")}
              </Button>
            </div>
          </div>
        </Card>
      ) : null}
      {trueEmpty ? <EmptyState icon="building" title={t("tenants.empty")} data-page-state="empty" /> : null}
      {filterEmpty ? <EmptyState icon="search" title={t("tenants.filterEmpty")} data-page-state="filtered_empty" /> : null}
      {filtered.length > 0 ? (
        <>
          <div className="flex items-center justify-between">
            <h2 className="text-sm font-semibold">{t("tenants.list")}</h2>
            <span className="text-xs text-muted-foreground">{metaN == null ? "—" : t("tenants.meta", { n: metaN })}</span>
          </div>
          <DataTable minWidth="720px">
            <THead>
              <Th>{t("tenants.col.name")}</Th>
              <Th>{t("tenants.col.slug")}</Th>
              <Th>{t("tenants.col.status")}</Th>
              <Th>{t("tenants.col.created")}</Th>
            </THead>
            <tbody>
              {filtered.map((row) => (
                <Tr key={row.id} selected={row.id === selected} onClick={() => void pick(row.id)}>
                  <Td className="font-semibold">
                    <span className="inline-flex flex-wrap items-center gap-2">
                      {tenantLabel(row)}
                      {row.master ? <StatusBadge status="info" label={t("tenants.badge.master")} /> : null}
                      {current?.id === row.id ? <StatusBadge status="default" label={t("tenants.badge.current")} /> : null}
                    </span>
                  </Td>
                  <Td className="text-muted-foreground">{row.id}</Td>
                  <Td>
                    <StatusBadge status={statusOf(row.status)} label={statusLabel(row.status)} />
                  </Td>
                  <Td className="tabular-nums text-muted-foreground">{formatWhen(row.created_at, na)}</Td>
                </Tr>
              ))}
            </tbody>
          </DataTable>
        </>
      ) : null}
      {showDetail && detail ? (
        <Card>
          <CardHeader
            icon="user"
            title={t("tenants.detail", { name: tenantLabel(detail) })}
            meta={detail.master ? t("tenants.badge.master") : detail.id}
          />
          <div className="flex flex-col gap-3 p-4">
            <div className="flex flex-wrap items-center gap-2">
              <StatusBadge status={statusOf(detail.status)} label={statusLabel(detail.status)} />
              {detail.master ? <StatusBadge status="info" label={t("tenants.badge.master")} /> : null}
              {current?.id === detail.id ? <StatusBadge status="default" label={t("tenants.badge.current")} /> : null}
            </div>
            <p className="text-sm text-muted-foreground">
              {t("tenants.created")}: {formatWhen(detail.created_at, na)}
            </p>
            <div className="flex flex-wrap gap-2">
              {detail.status === "active" && !detail.master ? (
                <Button
                  variant="ghost"
                  className="text-destructive"
                  disabled={blocked || Boolean(busy)}
                  onClick={() => {
                    setConfirm({ kind: "deactivate", tenant: detail });
                    setTyped("");
                    setOk("");
                    setDetailErr("");
                  }}
                >
                  {t("tenants.deactivate")}
                </Button>
              ) : null}
              {detail.status === "deactivated" ? (
                <Button variant="outline" disabled={blocked || Boolean(busy)} onClick={() => void activate()}>
                  {t("tenants.activate")}
                </Button>
              ) : null}
            </div>
            <div className="text-sm font-semibold">{t("tenants.members")}</div>
            <div className="flex flex-wrap items-end gap-2">
              <label className={labelClass + " min-w-[180px] flex-1"}>
                {t("tenants.subject")}
                <input
                  className={fieldClass + " mt-1"}
                  value={subject}
                  disabled={blocked}
                  onChange={(e) => setSubject(e.target.value)}
                  placeholder={t("tenants.subject")}
                  aria-label={t("tenants.subject")}
                  autoComplete="off"
                  spellCheck={false}
                />
              </label>
              <label className={labelClass}>
                {t("tenants.role")}
                <select className={selectClass + " mt-1"} value={role} disabled={blocked} onChange={(e) => setRole(e.target.value)} aria-label={t("tenants.role")}>
                  {ROLES.map((r) => (
                    <option key={r} value={r}>
                      {roleLabel(r)}
                    </option>
                  ))}
                </select>
              </label>
              <Button variant="outline" disabled={blocked || Boolean(busy) || !subject.trim()} onClick={() => void addMember()}>
                {t("tenants.addMember")}
              </Button>
            </div>
            {(detail.members || []).length === 0 ? <EmptyState icon="user" title={t("tenants.members.empty")} /> : null}
            {(detail.members || []).length > 0 ? (
              <DataTable minWidth="520px">
                <THead>
                  <Th>{t("tenants.col.subject")}</Th>
                  <Th>{t("tenants.col.role")}</Th>
                  <Th className="text-right" />
                </THead>
                <tbody>
                  {(detail.members || []).map((m) => (
                    <Tr key={m.id}>
                      <Td className="font-semibold">{m.subject}</Td>
                      <Td>
                        <select
                          className={selectClass}
                          value={m.role}
                          aria-label={t("tenants.role")}
                          disabled={blocked || Boolean(busy)}
                          onChange={(e) => void changeRole(m.id, e.target.value)}
                        >
                          {ROLES.map((r) => (
                            <option key={r} value={r}>
                              {roleLabel(r)}
                            </option>
                          ))}
                        </select>
                      </Td>
                      <Td>
                        <div className="flex justify-end">
                          <Button
                            variant="ghost"
                            size="sm"
                            className="text-destructive"
                            disabled={blocked || Boolean(busy)}
                            onClick={() => {
                              setConfirm({ kind: "remove", tenant: detail, member: m });
                              setTyped("");
                              setOk("");
                              setDetailErr("");
                            }}
                          >
                            {t("tenants.removeMember")}
                          </Button>
                        </div>
                      </Td>
                    </Tr>
                  ))}
                </tbody>
              </DataTable>
            ) : null}
          </div>
        </Card>
      ) : null}
    </PageChrome>
  );
}
