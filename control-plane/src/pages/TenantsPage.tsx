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
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { PageChrome } from "../ui/PageChrome";
import { PageStatus } from "../ui/PageStatus";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

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

  function statusTone(st: string): "positive" | "warning" | "neutral" {
    if (st === "active") return "positive";
    if (st === "deactivated") return "warning";
    return "neutral";
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
      icon="layers"
      title={t("tenants.title")}
      description={t("tenants.desc")}
      primary={
        <Button
          variant="primary"
          icon="plus"
          disabled={blocked || Boolean(busy) || !slug.trim() || !name.trim()}
          onClick={() => void createTenant()}
        >
          {t("tenants.create")}
        </Button>
      }
      refresh={
        <Button icon="refresh" iconGesture onClick={() => void load(selected)} disabled={loading || Boolean(busy)}>
          {t("common.refresh")}
        </Button>
      }
      filters={
        <input
          className="z-field"
          value={q}
          disabled={blocked}
          onChange={(e) => setQ(e.target.value)}
          placeholder={t("tenants.search")}
          aria-label={t("tenants.search")}
          autoComplete="off"
          style={{ minWidth: 220, flex: 1 }}
        />
      }
    >
      <Card>
        <CardHeader icon="shield" title={t("tenants.context")} />
        <p style={{ margin: 0, padding: "0 16px 14px", fontSize: 12.5, color: "var(--text-2)", lineHeight: 1.55 }}>
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
        <p role="status" style={{ margin: 0, fontSize: 12.5, color: "var(--green)" }}>
          {ok}
        </p>
      ) : null}
      <Card>
        <CardHeader icon="plus" title={t("tenants.create")} />
        <div style={{ padding: "0 16px 16px", display: "flex", gap: 8, flexWrap: "wrap", alignItems: "center" }}>
          <input
            className="z-field"
            value={slug}
            disabled={blocked}
            onChange={(e) => setSlug(e.target.value)}
            placeholder={t("tenants.slug")}
            aria-label={t("tenants.slug")}
            autoComplete="off"
            spellCheck={false}
            style={{ minWidth: 140 }}
          />
          <input
            className="z-field"
            value={name}
            disabled={blocked}
            onChange={(e) => setName(e.target.value)}
            placeholder={t("tenants.name")}
            aria-label={t("tenants.name")}
            autoComplete="off"
            style={{ minWidth: 180, flex: 1 }}
          />
        </div>
      </Card>
      {confirm && !blocked ? (
        <Card>
          <CardHeader icon="lock" title={confirm.kind === "deactivate" ? t("tenants.confirmDeactivateTitle") : t("tenants.confirmRemoveTitle")} />
          <div style={{ padding: "0 16px 16px", display: "flex", flexDirection: "column", gap: 10 }}>
            <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-2)" }}>
              {confirm.kind === "deactivate"
                ? t("tenants.confirmDeactivatePreview", { name: tenantLabel(confirm.tenant) })
                : t("tenants.confirmRemovePreview", { name: confirm.member?.subject || "" })}
            </p>
            <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>
              {confirm.kind === "deactivate" ? t("tenants.confirmHint") : t("tenants.confirmMemberHint")}
            </p>
            <input
              className="z-field"
              value={typed}
              onChange={(e) => setTyped(e.target.value)}
              placeholder={confirm.kind === "deactivate" ? t("tenants.confirmPlaceholder") : t("tenants.confirmMemberPlaceholder")}
              aria-label={confirm.kind === "deactivate" ? t("tenants.confirmPlaceholder") : t("tenants.confirmMemberPlaceholder")}
              autoComplete="off"
              spellCheck={false}
            />
            <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
              <Button variant="accent" disabled={!matched || Boolean(busy)} onClick={() => void submitConfirm()}>
                {confirm.kind === "deactivate" ? t("tenants.confirmDeactivate") : t("tenants.confirmRemove")}
              </Button>
              <Button
                variant="quiet"
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
      <Card>
        <CardHeader icon="list" title={t("tenants.list")} meta={metaN == null ? "—" : t("tenants.meta", { n: metaN })} />
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
            <span style={{ flex: 1.4 }}>{t("tenants.col.name")}</span>
            <span style={{ flex: 1.1 }}>{t("tenants.col.slug")}</span>
            <span style={{ flex: 0.9 }}>{t("tenants.col.status")}</span>
            <span style={{ flex: 1.2 }}>{t("tenants.col.created")}</span>
          </div>
          {trueEmpty ? <EmptyState data-page-state="empty">{t("tenants.empty")}</EmptyState> : null}
          {filterEmpty ? <EmptyState data-page-state="filtered_empty">{t("tenants.filterEmpty")}</EmptyState> : null}
          {filtered.map((row) => {
            const on = row.id === selected;
            return (
              <button
                key={row.id}
                type="button"
                onClick={() => void pick(row.id)}
                style={{
                  display: "flex",
                  alignItems: "center",
                  width: "100%",
                  padding: "11px 16px",
                  fontSize: 12.5,
                  border: "none",
                  borderBottom: "1px solid var(--border-soft)",
                  background: on ? "var(--accent-soft)" : "transparent",
                  color: "var(--text)",
                  textAlign: "left",
                  cursor: blocked ? "default" : "pointer",
                  gap: 8,
                }}
              >
                <span style={{ flex: 1.4, fontWeight: 600 }}>
                  {tenantLabel(row)}
                  {row.master ? (
                    <Badge tone="accent" style={{ marginLeft: 8 }}>
                      {t("tenants.badge.master")}
                    </Badge>
                  ) : null}
                  {current?.id === row.id ? (
                    <Badge tone="neutral" style={{ marginLeft: 6 }}>
                      {t("tenants.badge.current")}
                    </Badge>
                  ) : null}
                </span>
                <span style={{ flex: 1.1, color: "var(--text-2)" }}>{row.id}</span>
                <span style={{ flex: 0.9 }}>
                  <Badge tone={statusTone(row.status)}>{statusLabel(row.status)}</Badge>
                </span>
                <span style={{ flex: 1.2, color: "var(--text-3)", fontVariantNumeric: "tabular-nums" }}>{formatWhen(row.created_at, na)}</span>
              </button>
            );
          })}
        </TableScroll>
      </Card>
      {showDetail && detail ? (
        <Card>
          <CardHeader
            icon="user"
            title={t("tenants.detail", { name: tenantLabel(detail) })}
            meta={detail.master ? t("tenants.badge.master") : detail.id}
          />
          <div style={{ padding: "0 16px 16px", display: "flex", flexDirection: "column", gap: 12 }}>
            <div style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "center" }}>
              <Badge tone={statusTone(detail.status)}>{statusLabel(detail.status)}</Badge>
              {detail.master ? <Badge tone="accent">{t("tenants.badge.master")}</Badge> : null}
              {current?.id === detail.id ? <Badge tone="neutral">{t("tenants.badge.current")}</Badge> : null}
            </div>
            <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>
              {t("tenants.created")}: {formatWhen(detail.created_at, na)}
            </p>
            <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
              {detail.status === "active" && !detail.master ? (
                <Button
                  variant="quiet"
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
                <Button variant="quiet" disabled={blocked || Boolean(busy)} onClick={() => void activate()}>
                  {t("tenants.activate")}
                </Button>
              ) : null}
            </div>
            <div style={{ fontWeight: 600, fontSize: 13 }}>{t("tenants.members")}</div>
            <div style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "center" }}>
              <input
                className="z-field"
                value={subject}
                disabled={blocked}
                onChange={(e) => setSubject(e.target.value)}
                placeholder={t("tenants.subject")}
                aria-label={t("tenants.subject")}
                autoComplete="off"
                spellCheck={false}
                style={{ minWidth: 180, flex: 1 }}
              />
              <select className="z-field" value={role} disabled={blocked} onChange={(e) => setRole(e.target.value)} aria-label={t("tenants.role")}>
                {ROLES.map((r) => (
                  <option key={r} value={r}>
                    {roleLabel(r)}
                  </option>
                ))}
              </select>
              <Button variant="secondary" disabled={blocked || Boolean(busy) || !subject.trim()} onClick={() => void addMember()}>
                {t("tenants.addMember")}
              </Button>
            </div>
            <TableScroll>
              <div
                style={{
                  display: "flex",
                  padding: "8px 0",
                  borderBottom: "1px solid var(--border-soft)",
                  fontSize: 10,
                  fontWeight: 600,
                  letterSpacing: ".4px",
                  color: "var(--text-3)",
                }}
              >
                <span style={{ flex: 1.6 }}>{t("tenants.col.subject")}</span>
                <span style={{ flex: 1 }}>{t("tenants.col.role")}</span>
                <span style={{ flex: 1.1 }} />
              </div>
              {(detail.members || []).length === 0 ? <EmptyState>{t("tenants.members.empty")}</EmptyState> : null}
              {(detail.members || []).map((m) => (
                <div
                  key={m.id}
                  style={{
                    display: "flex",
                    alignItems: "center",
                    padding: "10px 0",
                    borderBottom: "1px solid var(--border-soft)",
                    gap: 8,
                    fontSize: 12.5,
                  }}
                >
                  <span style={{ flex: 1.6, fontWeight: 600 }}>{m.subject}</span>
                  <span style={{ flex: 1 }}>
                    <select
                      className="z-field"
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
                  </span>
                  <span style={{ flex: 1.1, display: "flex", justifyContent: "flex-end" }}>
                    <Button
                      variant="quiet"
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
                  </span>
                </div>
              ))}
            </TableScroll>
          </div>
        </Card>
      ) : null}
    </PageChrome>
  );
}
