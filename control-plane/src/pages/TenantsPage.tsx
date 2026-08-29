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
import { useI18n } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { SectionHeader } from "../ui/SectionHeader";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

type ConfirmKind = "deactivate" | "remove";

export function TenantsPage() {
  const { t } = useI18n();
  const [list, setList] = useState<TenantList>({ tenants: [] });
  const [detail, setDetail] = useState<Tenant | null>(null);
  const [selected, setSelected] = useState("");
  const [q, setQ] = useState("");
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState("");
  const [ok, setOk] = useState("");
  const [busy, setBusy] = useState("");
  const [slug, setSlug] = useState("");
  const [name, setName] = useState("");
  const [subject, setSubject] = useState("");
  const [role, setRole] = useState("member");
  const [confirm, setConfirm] = useState<{ kind: ConfirmKind; tenant: Tenant; member?: TenantMember } | null>(null);
  const [typed, setTyped] = useState("");
  const na = t("tenants.na");

  async function load(keepId = selected) {
    setLoading(true);
    try {
      const j = await tenantsApi.list(q);
      const leak = (j.tenants || []).some((row) => publicHasSecrets(row));
      setList(j);
      setErr(leak ? t("tenants.leak") : "");
      const id = keepId && j.tenants.some((r) => r.id === keepId) ? keepId : j.tenants[0]?.id || "";
      setSelected(id);
      if (id) {
        const d = await tenantsApi.get(id);
        setDetail(d);
        if (publicHasSecrets(d)) setErr(t("tenants.leak"));
      } else {
        setDetail(null);
      }
    } catch (e) {
      setErr(formatPublicError(e));
      setList({ tenants: [] });
      setDetail(null);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load(selected);
  }, [q]);

  const filtered = useMemo(() => filterTenants(list.tenants, q), [list.tenants, q]);
  const matched = confirm
    ? confirm.kind === "deactivate"
      ? tenantConfirmMatch(typed, confirm.tenant)
      : confirm.member
        ? memberConfirmMatch(typed, confirm.member)
        : false
    : false;

  async function createTenant() {
    setBusy("create");
    setOk("");
    try {
      const row = await tenantsApi.create(slug.trim(), name.trim());
      setSlug("");
      setName("");
      setOk(t("tenants.createOk"));
      setErr("");
      await load(row.id);
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  async function pick(id: string) {
    setSelected(id);
    setConfirm(null);
    setTyped("");
    setOk("");
    try {
      const d = await tenantsApi.get(id);
      setDetail(d);
      if (publicHasSecrets(d)) setErr(t("tenants.leak"));
    } catch (e) {
      setErr(formatPublicError(e));
      setDetail(null);
    }
  }

  async function activate() {
    if (!detail) return;
    setBusy("status");
    try {
      const row = await tenantsApi.setStatus(detail.id, "active");
      setOk(t("tenants.activateOk"));
      setErr("");
      await load(row.id);
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  async function addMember() {
    if (!detail) return;
    setBusy("member");
    try {
      const row = await tenantsApi.addMember(detail.id, subject.trim(), role);
      setSubject("");
      setRole("member");
      setOk(t("tenants.memberOk"));
      setErr("");
      setDetail(row);
      await load(row.id);
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  async function changeRole(mid: string, next: string) {
    if (!detail) return;
    setBusy("role:" + mid);
    try {
      const row = await tenantsApi.setMemberRole(detail.id, mid, next);
      setOk(t("tenants.roleOk"));
      setErr("");
      setDetail(row);
    } catch (e) {
      setErr(formatPublicError(e));
    } finally {
      setBusy("");
    }
  }

  async function submitConfirm() {
    if (!confirm) return;
    if (confirm.kind === "deactivate" && !tenantConfirmMatch(typed, confirm.tenant)) {
      setErr(t("tenants.mismatch"));
      return;
    }
    if (confirm.kind === "remove" && confirm.member && !memberConfirmMatch(typed, confirm.member)) {
      setErr(t("tenants.mismatch"));
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
      setErr("");
      setConfirm(null);
      setTyped("");
      await load(confirm.tenant.id);
    } catch (e) {
      setErr(formatPublicError(e));
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

  const current = list.current;
  const master = list.master;
  const empty = !loading && !err && filtered.length === 0;

  return (
    <div style={{ padding: "14px 22px 40px", display: "flex", flexDirection: "column", gap: 14 }}>
      <SectionHeader
        icon="layers"
        title={t("tenants.title")}
        description={t("tenants.desc")}
        actions={
          <Button icon="refresh" iconGesture onClick={() => void load(selected)} disabled={loading || Boolean(busy)}>
            {t("common.refresh")}
          </Button>
        }
      />
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
      {loading ? <StatusLine kind="loading" /> : null}
      {err ? <StatusLine kind="error">{err}</StatusLine> : null}
      {ok && !err ? (
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
            onChange={(e) => setName(e.target.value)}
            placeholder={t("tenants.name")}
            aria-label={t("tenants.name")}
            autoComplete="off"
            style={{ minWidth: 180, flex: 1 }}
          />
          <Button variant="accent" disabled={Boolean(busy) || !slug.trim() || !name.trim()} onClick={() => void createTenant()}>
            {t("common.create")}
          </Button>
        </div>
      </Card>
      {confirm ? (
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
        <CardHeader icon="list" title={t("tenants.list")} meta={t("tenants.meta", { n: filtered.length })} />
        <div style={{ padding: "0 16px 10px" }}>
          <input
            className="z-field"
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder={t("tenants.search")}
            aria-label={t("tenants.search")}
            autoComplete="off"
            style={{ width: "100%" }}
          />
        </div>
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
          {empty ? <EmptyState>{q.trim() ? t("tenants.filterEmpty") : t("tenants.empty")}</EmptyState> : null}
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
                  cursor: "pointer",
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
      {detail ? (
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
                  disabled={Boolean(busy)}
                  onClick={() => {
                    setConfirm({ kind: "deactivate", tenant: detail });
                    setTyped("");
                    setOk("");
                    setErr("");
                  }}
                >
                  {t("tenants.deactivate")}
                </Button>
              ) : null}
              {detail.status === "deactivated" ? (
                <Button variant="quiet" disabled={Boolean(busy)} onClick={() => void activate()}>
                  {t("tenants.activate")}
                </Button>
              ) : null}
            </div>
            <div style={{ fontWeight: 600, fontSize: 13 }}>{t("tenants.members")}</div>
            <div style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "center" }}>
              <input
                className="z-field"
                value={subject}
                onChange={(e) => setSubject(e.target.value)}
                placeholder={t("tenants.subject")}
                aria-label={t("tenants.subject")}
                autoComplete="off"
                spellCheck={false}
                style={{ minWidth: 180, flex: 1 }}
              />
              <select className="z-field" value={role} onChange={(e) => setRole(e.target.value)} aria-label={t("tenants.role")}>
                {ROLES.map((r) => (
                  <option key={r} value={r}>
                    {roleLabel(r)}
                  </option>
                ))}
              </select>
              <Button variant="secondary" disabled={Boolean(busy) || !subject.trim()} onClick={() => void addMember()}>
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
                      disabled={Boolean(busy)}
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
                      disabled={Boolean(busy)}
                      onClick={() => {
                        setConfirm({ kind: "remove", tenant: detail, member: m });
                        setTyped("");
                        setOk("");
                        setErr("");
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
    </div>
  );
}
