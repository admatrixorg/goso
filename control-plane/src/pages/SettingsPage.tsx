import { useEffect, useRef, useState, type ReactNode } from "react";
import { api, TENANT_STORAGE_KEY, type TenantInfo } from "../api/client";
import { BackupPanel } from "./BackupPanel";
import { pairingApi, type PairingIssued } from "../api/pairing";
import { crmOrgId } from "../api/crm";
import { isDemoMode } from "../demo/mode";
import {
  settingsApi,
  type GatewayConfig,
  type SettingsAccount,
  type SettingsNick,
  type SettingsQuota,
  type SettingsRole,
  type SettingsTemplate,
  type SettingsUser,
} from "../api/settings";
import {
  editableValues,
  emptyGatewayForm,
  fieldValue,
  formFromSnapshot,
  isFieldEditable,
  isFieldEnvOwned,
  publicHasSecrets,
  settingsConflictKind,
  validateGatewayForm,
  type GatewayField,
  type GatewayForm,
} from "../api/settings-ops";
import { useI18n, type MsgKey } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { Icon, type IconName } from "../ui/Icon";
import { StatusLine, formatPublicError } from "../ui/StatusLine";

type PageId = "account" | "users" | "roles" | "nicks" | "quotas" | "templates" | "billing" | "gateway" | "backup" | "pairing" | "theme";

const FIELD_KEYS: Record<string, MsgKey> = {
  port: "settings.field.port",
  host: "settings.field.host",
  env: "settings.field.env",
  log_level: "settings.field.log_level",
  token_set: "settings.field.token_set",
  view_token_set: "settings.field.view_token_set",
  master_key_set: "settings.field.master_key_set",
  context_dir: "settings.field.context_dir",
  workspace: "settings.field.workspace",
  kg_extract: "settings.field.kg_extract",
  cache_mode: "settings.field.cache_mode",
  heartbeat: "settings.field.heartbeat",
  heartbeat_interval_sec: "settings.field.heartbeat_interval_sec",
  day_limit: "settings.field.day_limit",
  enabled: "settings.field.enabled",
  injection: "settings.field.injection",
  ssrf: "settings.field.ssrf",
  otel_set: "settings.field.otel_set",
  database_url_set: "settings.field.database_url_set",
  multi_tenant: "settings.field.multi_tenant",
  skills_dir: "settings.field.skills_dir",
  vault_dir: "settings.field.vault_dir",
};

export function SettingsPage({ dark, onToggleTheme }: { dark: boolean; onToggleTheme: () => void }) {
  const { t } = useI18n();
  const [page, setPage] = useState<PageId>("account");
  const [org, setOrg] = useState(crmOrgId);
  const [err, setErr] = useState("");
  const [users, setUsers] = useState<SettingsUser[]>([]);
  const [roles, setRoles] = useState<SettingsRole[]>([]);
  const [nicks, setNicks] = useState<SettingsNick[]>([]);
  const [quota, setQuota] = useState<SettingsQuota | null>(null);
  const [templates, setTemplates] = useState<SettingsTemplate[]>([]);
  const [account, setAccount] = useState<SettingsAccount | null>(null);
  const [developing, setDeveloping] = useState("");

  const [pairing, setPairing] = useState<PairingIssued | null>(null);
  const [pairingCopied, setPairingCopied] = useState(false);
  const [pairingBusy, setPairingBusy] = useState(false);
  const pairingInFlight = useRef(false);

  const [userName, setUserName] = useState("");
  const [userEmail, setUserEmail] = useState("");
  const [userRole, setUserRole] = useState("");
  const [roleName, setRoleName] = useState("");
  const [roleFlags, setRoleFlags] = useState("{}");
  const [nickName, setNickName] = useState("");
  const [cap, setCap] = useState("0");
  const [tplName, setTplName] = useState("");
  const [tplBody, setTplBody] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [gwTenant, setGwTenant] = useState<TenantInfo>({ tenant: "default", multi_tenant: false });
  const [tenantInput, setTenantInput] = useState("default");
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState("");
  const [gw, setGw] = useState<GatewayConfig | null>(null);
  const [gwForm, setGwForm] = useState<GatewayForm>(emptyGatewayForm);

  const menu: { group: string; items: { id: PageId; label: string; ic: IconName }[] }[] = [
    {
      group: t("settings.group.account"),
      items: [{ id: "account", label: t("settings.account"), ic: "user" }],
    },
    {
      group: t("settings.group.team"),
      items: [
        { id: "users", label: t("settings.users"), ic: "friends" },
        { id: "roles", label: t("settings.roles"), ic: "shield" },
        { id: "nicks", label: t("settings.nicks"), ic: "tag" },
      ],
    },
    {
      group: t("settings.group.messaging"),
      items: [
        { id: "quotas", label: t("settings.quotas"), ic: "timer" },
        { id: "templates", label: t("settings.templates"), ic: "doc" },
      ],
    },
    {
      group: t("settings.group.system"),
      items: [
        { id: "billing", label: t("settings.billing"), ic: "flag" },
        { id: "gateway", label: t("settings.gateway"), ic: "gear" },
        { id: "backup", label: t("settings.backup"), ic: "download" },
        { id: "pairing", label: t("settings.pairing"), ic: "device" },
        { id: "theme", label: t("settings.theme"), ic: "sun" },
      ],
    },
  ];

  function mapErr(e: unknown): string {
    const kind = settingsConflictKind(e);
    if (kind === "env_owned") return t("settings.envOwnedConflict");
    if (kind === "conflict") return t("settings.conflict");
    return formatPublicError(e);
  }

  async function load() {
    const id = org.trim() || crmOrgId();
    setErr("");
    setDeveloping("");
    if (page === "theme" || page === "pairing" || page === "backup") return;
    setLoading(true);
    try {
      if (page === "users") setUsers(await settingsApi.listUsers(id));
      else if (page === "roles") setRoles(await settingsApi.listRoles(id));
      else if (page === "nicks") setNicks(await settingsApi.listNicks(id));
      else if (page === "quotas") {
        const q = await settingsApi.getQuota(id);
        setQuota(q);
        setCap(String(q.dailySendCap ?? 0));
      } else if (page === "templates") setTemplates(await settingsApi.listTemplates(id));
      else if (page === "account") {
        const a = await settingsApi.getAccount(id);
        setAccount(a);
        setDisplayName(a.displayName ?? "");
        try {
          const info = await api.tenant();
          setGwTenant(info);
          try {
            const stored = localStorage.getItem(TENANT_STORAGE_KEY);
            setTenantInput((stored && stored.trim()) || info.tenant || "default");
          } catch {
            setTenantInput(info.tenant || "default");
          }
        } catch {
          setGwTenant({ tenant: "default", multi_tenant: false });
        }
      } else if (page === "billing") {
        const d = await settingsApi.billing(id);
        setDeveloping(d.status || "developing");
      } else if (page === "gateway") {
        const cfg = await settingsApi.getGateway();
        if (publicHasSecrets(cfg)) {
          setErr(t("settings.secretHint"));
        }
        setGw(cfg);
        setGwForm(formFromSnapshot(cfg));
      }
    } catch (e) {
      setErr(mapErr(e));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    setSaved("");
    if (page === "theme" || page === "pairing" || page === "backup") return;
    void load();
  }, [page, org]);

  useEffect(() => {
    if (page !== "pairing") {
      setPairing(null);
      setPairingCopied(false);
    }
  }, [page]);

  async function run(fn: () => Promise<unknown>) {
    if (saving) return;
    setSaving(true);
    setSaved("");
    try {
      setErr("");
      const ok = await fn();
      if (ok === false) return;
      await load();
      setSaved(t("settings.saved"));
    } catch (e) {
      setErr(mapErr(e));
    } finally {
      setSaving(false);
    }
  }

  async function saveGateway() {
    if (saving) return;
    const values = editableValues(gwForm, gw);
    const invalid = validateGatewayForm(gwForm, values);
    if (invalid) {
      setErr(t(invalid as MsgKey));
      setSaved("");
      return;
    }
    await run(() => settingsApi.putGateway({ updated_at: gw?.updated_at || "", values }));
  }

  return (
    <div className="z-split-stack">
      <div className="z-split-rail" style={{ background: "var(--card)", borderRight: "1px solid var(--border)", overflowY: "auto", padding: "14px 10px" }}>
        <div style={{ display: "flex", gap: 8, alignItems: "center", fontWeight: 700, fontSize: 15, padding: "0 8px 10px" }}>{t("settings.title")}</div>
        {menu.map((g) => (
          <div key={g.group}>
            <div style={{ fontSize: 10, fontWeight: 700, letterSpacing: ".7px", color: "var(--text-3)", padding: "10px 12px 4px" }}>{g.group}</div>
            {g.items.map((i) => {
              const on = page === i.id;
              return (
                <button
                  key={i.id}
                  type="button"
                  onClick={() => setPage(i.id)}
                  style={{
                    width: "100%",
                    border: "none",
                    borderRadius: 8,
                    padding: "7px 12px",
                    fontSize: 13,
                    display: "flex",
                    gap: 9,
                    alignItems: "center",
                    background: on ? "var(--accent-soft)" : "transparent",
                    color: on ? "var(--accent)" : "var(--text-2)",
                    fontWeight: on ? 600 : 400,
                    textAlign: "left",
                  }}
                >
                  <Icon name={i.ic} size={14} />
                  {i.label}
                </button>
              );
            })}
          </div>
        ))}
      </div>
      <div style={{ flex: 1, overflowY: "auto", padding: "16px 26px", display: "flex", flexDirection: "column", gap: 12 }}>
        {page !== "theme" && page !== "backup" && page !== "gateway" && page !== "pairing" ? (
          <div style={{ display: "flex", gap: 8, alignItems: "center", flexWrap: "wrap" }}>
            <span style={{ fontSize: 12, color: "var(--text-3)" }}>{t("common.org")}</span>
            <input className="z-field" style={{ minWidth: 0, flex: 1 }} value={org} onChange={(e) => setOrg(e.target.value)} aria-label="CRM org id" />
            <Button icon="refresh" iconGesture onClick={() => void load()}>
              {t("common.refresh")}
            </Button>
          </div>
        ) : null}
        {page !== "backup" && loading ? <StatusLine kind="loading" /> : null}
        {page !== "backup" && err ? <StatusLine kind="error">{err}</StatusLine> : null}
        {saved && !err ? <p role="status" style={{ color: "var(--green)", fontSize: 12.5, margin: 0 }}>{saved}</p> : null}

        {page === "account" && (
          <>
            <div style={{ fontSize: 21, fontWeight: 700 }}>{t("settings.account")}</div>
            <div style={{ fontSize: 12.5, color: "var(--text-3)" }}>{t("settings.account.desc")}</div>
            <Card style={{ padding: 16, display: "flex", gap: 8, flexWrap: "wrap", alignItems: "center" }}>
              <input
                className="z-field"
                placeholder={t("settings.account.displayName")}
                value={displayName}
                onChange={(e) => setDisplayName(e.target.value)}
                style={{ minWidth: 240, flex: 1 }}
              />
              <Button variant="primary" disabled={saving} onClick={() => void run(() => settingsApi.putAccount({ displayName: displayName.trim() }, org))}>
                {t("common.save")}
              </Button>
            </Card>
            {account?.orgId ? (
              <div style={{ fontSize: 12, color: "var(--text-3)" }}>
                org {account.orgId}
                {account.displayName ? ` · ${account.displayName}` : ""}
              </div>
            ) : null}
            <Card style={{ padding: 16, display: "flex", flexDirection: "column", gap: 8 }}>
              <div style={{ fontWeight: 700, fontSize: 14 }}>{t("settings.tenant")}</div>
              <div style={{ fontSize: 12.5, color: "var(--text-3)" }}>{t("settings.tenant.desc")}</div>
              <div style={{ fontSize: 13 }}>
                {t("settings.tenant.current")}: <strong>{isDemoMode() || !gwTenant.multi_tenant ? "default" : gwTenant.tenant}</strong>
              </div>
              <div style={{ fontSize: 12.5, color: "var(--text-3)" }}>
                {t("settings.tenant.master")}: <strong>{gwTenant.master_id || "default"}</strong>
                {gwTenant.master ? ` · ${t("tenants.badge.master")}` : ""}
              </div>
              {gwTenant.multi_tenant && !isDemoMode() ? (
                <div style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "center" }}>
                  <input
                    className="z-field"
                    aria-label={t("settings.tenant.header")}
                    value={tenantInput}
                    onChange={(e) => setTenantInput(e.target.value)}
                    style={{ minWidth: 180, flex: 1 }}
                  />
                  <Button
                    variant="primary"
                    onClick={() => {
                      const v = tenantInput.trim() || "default";
                      try {
                        localStorage.setItem(TENANT_STORAGE_KEY, v);
                      } catch {
                        /* ignore */
                      }
                      setTenantInput(v);
                      void load();
                    }}
                  >
                    {t("common.save")}
                  </Button>
                  <span style={{ fontSize: 12, color: "var(--text-3)" }}>{t("settings.tenant.multi")}</span>
                </div>
              ) : (
                <div style={{ fontSize: 12, color: "var(--text-3)" }}>
                  {isDemoMode() ? t("settings.tenant.readonly") : t("settings.tenant.single")}
                </div>
              )}
            </Card>
            <Card>
              <CardHeader icon="lock" title={t("settings.account")} />
              <div style={{ padding: 16, fontSize: 13, color: "var(--text-2)", lineHeight: 1.6 }}>{t("settings.secretHint")}</div>
            </Card>
          </>
        )}

        {page === "users" && (
          <>
            <div style={{ fontSize: 21, fontWeight: 700 }}>{t("settings.users")}</div>
            <div style={{ fontSize: 12.5, color: "var(--text-3)" }}>{t("settings.users.desc")}</div>
            <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
              <input className="z-field" placeholder={t("common.name")} value={userName} onChange={(e) => setUserName(e.target.value)} />
              <input className="z-field" placeholder={t("common.email")} value={userEmail} onChange={(e) => setUserEmail(e.target.value)} />
              <input className="z-field" placeholder={t("settings.users.roleId")} value={userRole} onChange={(e) => setUserRole(e.target.value)} />
              <Button
                variant="primary"
                icon="plus"
                onClick={() =>
                  void run(async () => {
                    if (!userName.trim()) {
                      setErr(t("settings.needName"));
                      return false;
                    }
                    await settingsApi.createUser({ name: userName.trim(), email: userEmail.trim(), roleId: userRole.trim() }, org);
                    setUserName("");
                    setUserEmail("");
                    setUserRole("");
                  })
                }
              >
                {t("common.create")}
              </Button>
            </div>
            <Card>
              <CardHeader icon="friends" title={t("settings.users")} meta={String(users.length)} />
              <TableScroll>
              <Row head>
                <span style={{ flex: 1.4 }}>{t("settings.col.name")}</span>
                <span style={{ flex: 1.6 }}>{t("settings.col.email")}</span>
                <span style={{ flex: 1 }}>{t("settings.col.role")}</span>
                <span style={{ flex: 0.8 }}>{t("settings.col.active")}</span>
                <span style={{ width: 88 }} />
              </Row>
              {users.map((u) => (
                <Row key={u.id}>
                  <span style={{ flex: 1.4, fontWeight: 600 }}>{u.name}</span>
                  <span style={{ flex: 1.6, color: "var(--text-2)" }}>{u.email || "—"}</span>
                  <span style={{ flex: 1, color: "var(--text-3)" }}>{u.roleId || "—"}</span>
                  <span style={{ flex: 0.8 }}>{u.active ? t("common.active") : t("common.inactive")}</span>
                  <span style={{ width: 88, display: "flex", gap: 4, justifyContent: "flex-end" }}>
                    <Button
                      variant="quiet"
                      style={{ padding: "4px 8px" }}
                      onClick={() => void run(() => settingsApi.patchUser(u.id, { active: !u.active }, org))}
                    >
                      {u.active ? t("common.inactive") : t("common.active")}
                    </Button>
                    <Button variant="quiet" style={{ padding: "4px 8px" }} onClick={() => void run(() => settingsApi.deleteUser(u.id, org))}>
                      {t("common.delete")}
                    </Button>
                  </span>
                </Row>
              ))}
              {users.length === 0 ? <EmptyState>{t("settings.users.empty")}</EmptyState> : null}
              </TableScroll>
            </Card>
          </>
        )}

        {page === "roles" && (
          <>
            <div style={{ fontSize: 21, fontWeight: 700 }}>{t("settings.roles")}</div>
            <div style={{ fontSize: 12.5, color: "var(--text-3)" }}>{t("settings.roles.desc")}</div>
            <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
              <input className="z-field" placeholder={t("common.name")} value={roleName} onChange={(e) => setRoleName(e.target.value)} />
              <input className="z-field" placeholder={t("settings.roles.flags")} value={roleFlags} onChange={(e) => setRoleFlags(e.target.value)} style={{ minWidth: 220, flex: 1 }} />
              <Button
                variant="primary"
                icon="plus"
                onClick={() =>
                  void run(async () => {
                    if (!roleName.trim()) {
                      setErr(t("settings.needName"));
                      return false;
                    }
                    let flags: Record<string, unknown> = {};
                    try {
                      flags = JSON.parse(roleFlags || "{}") as Record<string, unknown>;
                    } catch {
                      setErr(t("settings.roles.flags"));
                      return false;
                    }
                    await settingsApi.createRole({ name: roleName.trim(), flags }, org);
                    setRoleName("");
                    setRoleFlags("{}");
                  })
                }
              >
                {t("common.create")}
              </Button>
            </div>
            <Card>
              <CardHeader icon="shield" title={t("settings.roles")} meta={String(roles.length)} />
              <TableScroll>
              <Row head>
                <span style={{ flex: 1.4 }}>{t("settings.col.name")}</span>
                <span style={{ flex: 3 }}>{t("settings.col.flags")}</span>
              </Row>
              {roles.map((r) => (
                <Row key={r.id}>
                  <span style={{ flex: 1.4, fontWeight: 600 }}>{r.name}</span>
                  <span style={{ flex: 3, color: "var(--text-2)", fontFamily: "var(--font-mono, ui-monospace)", fontSize: 12 }}>{JSON.stringify(r.flags)}</span>
                </Row>
              ))}
              {roles.length === 0 ? <EmptyState>{t("settings.roles.empty")}</EmptyState> : null}
              </TableScroll>
            </Card>
          </>
        )}

        {page === "nicks" && (
          <>
            <div style={{ fontSize: 21, fontWeight: 700 }}>{t("settings.nicks")}</div>
            <div style={{ fontSize: 12.5, color: "var(--text-3)" }}>{t("settings.nicks.desc")}</div>
            <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
              <input className="z-field" placeholder={t("settings.account.displayName")} value={nickName} onChange={(e) => setNickName(e.target.value)} />
              <Button
                variant="primary"
                icon="plus"
                onClick={() =>
                  void run(async () => {
                    if (!nickName.trim()) {
                      setErr(t("settings.needName"));
                      return false;
                    }
                    await settingsApi.createNick({ displayName: nickName.trim() }, org);
                    setNickName("");
                  })
                }
              >
                {t("common.create")}
              </Button>
            </div>
            <Card>
              <CardHeader icon="tag" title={t("settings.nicks")} meta={String(nicks.length)} />
              <TableScroll>
              {nicks.map((n) => (
                <Row key={n.id}>
                  <span style={{ flex: 1, fontWeight: 600 }}>{n.displayName}</span>
                  <span style={{ color: "var(--text-3)", fontVariantNumeric: "tabular-nums" }}>{n.id}</span>
                </Row>
              ))}
              {nicks.length === 0 ? <EmptyState>{t("settings.nicks.empty")}</EmptyState> : null}
              </TableScroll>
            </Card>
          </>
        )}

        {page === "quotas" && (
          <>
            <div style={{ fontSize: 21, fontWeight: 700 }}>{t("settings.quotas")}</div>
            <div style={{ fontSize: 12.5, color: "var(--text-3)" }}>{t("settings.quotas.desc")}</div>
            <Card style={{ padding: 16, display: "flex", gap: 8, alignItems: "center", flexWrap: "wrap" }}>
              <label style={{ fontSize: 13, color: "var(--text-2)" }}>{t("settings.quotas.daily")}</label>
              <input className="z-field" type="number" min={0} value={cap} onChange={(e) => setCap(e.target.value)} style={{ width: 140 }} />
              <Button
                variant="primary"
                disabled={saving}
                onClick={() =>
                  void run(async () => {
                    if (!/^\d+$/.test(cap.trim())) {
                      setErr(t("settings.invalidQuota"));
                      return false;
                    }
                    await settingsApi.putQuota({ dailySendCap: Number.parseInt(cap, 10) || 0 }, org);
                  })
                }
              >
                {t("common.save")}
              </Button>
              {quota ? (
                <span style={{ fontSize: 12, color: "var(--text-3)" }}>
                  {quota.orgId} · {quota.dailySendCap}
                </span>
              ) : null}
            </Card>
          </>
        )}

        {page === "templates" && (
          <>
            <div style={{ fontSize: 21, fontWeight: 700 }}>{t("settings.templates")}</div>
            <div style={{ fontSize: 12.5, color: "var(--text-3)" }}>{t("settings.templates.desc")}</div>
            <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
              <input className="z-field" placeholder={t("common.name")} value={tplName} onChange={(e) => setTplName(e.target.value)} />
              <input className="z-field" placeholder={t("settings.templates.body")} value={tplBody} onChange={(e) => setTplBody(e.target.value)} style={{ flex: 1, minWidth: 220 }} />
              <Button
                variant="primary"
                icon="plus"
                onClick={() =>
                  void run(async () => {
                    if (!tplName.trim()) {
                      setErr(t("settings.needName"));
                      return false;
                    }
                    await settingsApi.createTemplate({ name: tplName.trim(), body: tplBody }, org);
                    setTplName("");
                    setTplBody("");
                  })
                }
              >
                {t("common.create")}
              </Button>
            </div>
            <Card>
              <CardHeader icon="doc" title={t("settings.templates")} meta={String(templates.length)} />
              <TableScroll>
              <Row head>
                <span style={{ flex: 1.4 }}>{t("settings.col.name")}</span>
                <span style={{ flex: 3 }}>{t("settings.col.body")}</span>
                <span style={{ width: 72 }} />
              </Row>
              {templates.map((tpl) => (
                <Row key={tpl.id}>
                  <span style={{ flex: 1.4, fontWeight: 600 }}>{tpl.name}</span>
                  <span style={{ flex: 3, color: "var(--text-2)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{tpl.body}</span>
                  <span style={{ width: 72, textAlign: "right" }}>
                    <Button variant="quiet" style={{ padding: "4px 8px" }} onClick={() => void run(() => settingsApi.deleteTemplate(tpl.id, org))}>
                      {t("common.delete")}
                    </Button>
                  </span>
                </Row>
              ))}
              {templates.length === 0 ? <EmptyState>{t("settings.templates.empty")}</EmptyState> : null}
              </TableScroll>
            </Card>
          </>
        )}

        {page === "billing" && (
          <>
            <div style={{ fontSize: 21, fontWeight: 700 }}>{t("settings.billing")}</div>
            <Card style={{ padding: 18 }}>
              <div style={{ fontSize: 15, fontWeight: 600, marginBottom: 8 }}>{t("common.developing")}</div>
              <div style={{ fontSize: 12.5, color: "var(--text-3)", lineHeight: 1.6 }}>{t("settings.developingHint")}</div>
              {developing ? (
                <div style={{ marginTop: 10, fontSize: 12, color: "var(--text-2)", fontFamily: "var(--font-mono, ui-monospace)" }}>status: {developing}</div>
              ) : null}
            </Card>
          </>
        )}

        {page === "gateway" && (
          <>
            <div style={{ fontSize: 21, fontWeight: 700 }}>{t("settings.gateway")}</div>
            <div style={{ fontSize: 12.5, color: "var(--text-3)" }}>{t("settings.gateway.desc")}</div>
            <div style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "center" }}>
              <Button icon="refresh" iconGesture onClick={() => void load()}>
                {t("common.refresh")}
              </Button>
              <Button variant="primary" disabled={saving || loading} onClick={() => void saveGateway()}>
                {t("common.save")}
              </Button>
            </div>
            <GatewayCard title={t("settings.gateway.server")} icon="gear">
              <ReadRow t={t} field={gw?.server?.port} />
              <ReadRow t={t} field={gw?.server?.host} />
              <ReadRow t={t} field={gw?.server?.env} />
              <EditRow
                t={t}
                field={gw?.server?.log_level}
                value={gwForm.log_level}
                onChange={(v) => setGwForm({ ...gwForm, log_level: v })}
                options={["debug", "info", "warn", "error"]}
              />
            </GatewayCard>
            <GatewayCard title={t("settings.gateway.auth")} icon="lock">
              <ReadRow t={t} field={gw?.auth?.token_set} boolean />
              <ReadRow t={t} field={gw?.auth?.view_token_set} boolean />
              <ReadRow t={t} field={gw?.auth?.master_key_set} boolean />
              <div style={{ padding: "8px 16px 12px", fontSize: 12.5, color: "var(--text-3)", lineHeight: 1.5 }}>{t("settings.secretHint")}</div>
            </GatewayCard>
            <GatewayCard title={t("settings.gateway.behavior")} icon="pulse">
              <ReadRow t={t} field={gw?.behavior?.context_dir} />
              <ReadRow t={t} field={gw?.behavior?.workspace} />
              <EditRow
                t={t}
                field={gw?.behavior?.kg_extract}
                value={gwForm.kg_extract}
                onChange={(v) => setGwForm({ ...gwForm, kg_extract: v })}
                options={["on", "off"]}
              />
              <EditRow
                t={t}
                field={gw?.behavior?.cache_mode}
                value={gwForm.cache_mode}
                onChange={(v) => setGwForm({ ...gwForm, cache_mode: v })}
                options={["", "none", "full"]}
              />
              <EditRow
                t={t}
                field={gw?.behavior?.heartbeat}
                value={gwForm.heartbeat}
                onChange={(v) => setGwForm({ ...gwForm, heartbeat: v })}
                options={["on", "off"]}
              />
              <ReadRow t={t} field={gw?.behavior?.heartbeat_interval_sec} />
            </GatewayCard>
            <GatewayCard title={t("settings.gateway.quota")} icon="timer">
              <EditRow
                t={t}
                field={gw?.quota?.day_limit}
                value={gwForm.quota_day}
                onChange={(v) => setGwForm({ ...gwForm, quota_day: v })}
                number
              />
              <ReadRow t={t} field={gw?.quota?.enabled} boolean />
            </GatewayCard>
            <GatewayCard title={t("settings.gateway.tools")} icon="build">
              <EditRow
                t={t}
                field={gw?.tools?.injection}
                value={gwForm.injection}
                onChange={(v) => setGwForm({ ...gwForm, injection: v })}
                options={["log", "block"]}
              />
              <EditRow
                t={t}
                field={gw?.tools?.ssrf}
                value={gwForm.ssrf}
                onChange={(v) => setGwForm({ ...gwForm, ssrf: v })}
                options={["on", "off"]}
              />
            </GatewayCard>
            <GatewayCard title={t("settings.gateway.integrations")} icon="layers">
              <ReadRow t={t} field={gw?.integrations?.otel_set} boolean />
              <ReadRow t={t} field={gw?.integrations?.database_url_set} boolean />
              <ReadRow t={t} field={gw?.integrations?.multi_tenant} boolean />
              <ReadRow t={t} field={gw?.integrations?.skills_dir} />
              <ReadRow t={t} field={gw?.integrations?.vault_dir} />
            </GatewayCard>
          </>
        )}

        {page === "backup" ? <BackupPanel /> : null}

        {page === "pairing" && (
          <>
            <div style={{ fontSize: 21, fontWeight: 700 }}>{t("settings.pairing")}</div>
            <div style={{ fontSize: 12.5, color: "var(--text-3)" }}>{t("settings.pairing.desc")}</div>
            <div style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "center" }}>
              <Button
                variant="primary"
                icon="plus"
                disabled={pairingBusy || isDemoMode()}
                onClick={() => {
                  if (pairingInFlight.current || pairingBusy || isDemoMode()) return;
                  pairingInFlight.current = true;
                  setPairingBusy(true);
                  setPairingCopied(false);
                  setErr("");
                  void pairingApi
                    .create()
                    .then((issued) => setPairing(issued))
                    .catch((e) => setErr(String(e)))
                    .finally(() => {
                      pairingInFlight.current = false;
                      setPairingBusy(false);
                    });
                }}
              >
                {t("settings.pairing.generate")}
              </Button>
            </div>
            {isDemoMode() ? (
              <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-3)" }}>{t("settings.pairing.demo")}</p>
            ) : null}
            <Card>
              <CardHeader icon="lock" title={t("settings.pairing")} />
              {pairing ? (
                <div style={{ padding: "12px 16px", display: "flex", flexDirection: "column", gap: 10, fontSize: 12.5 }}>
                  <p style={{ margin: 0, color: "var(--text-3)" }}>{t("settings.pairing.once")}</p>
                  <div style={{ display: "flex", gap: 8, alignItems: "center", flexWrap: "wrap" }}>
                    <code style={{ flex: 1, fontSize: 18, letterSpacing: "0.12em", fontWeight: 700 }}>{pairing.code}</code>
                    <Button
                      onClick={() => {
                        void navigator.clipboard.writeText(pairing.code).then(
                          () => setPairingCopied(true),
                          (e) => setErr(String(e)),
                        );
                      }}
                      style={{ padding: "4px 10px" }}
                    >
                      {t("common.copy")}
                    </Button>
                  </div>
                  {pairingCopied ? <p style={{ margin: 0, color: "var(--green)" }}>{t("settings.pairing.copied")}</p> : null}
                  <div style={{ color: "var(--text-3)" }}>{t("settings.pairing.expires", { at: pairing.expires_at })}</div>
                </div>
              ) : (
                <EmptyState>{t("settings.pairing.empty")}</EmptyState>
              )}
            </Card>
            <p style={{ margin: 0, fontSize: 12, color: "var(--text-3)", lineHeight: 1.5 }}>{t("settings.pairing.hint")}</p>
          </>
        )}

        {page === "theme" && (
          <>
            <div style={{ fontSize: 21, fontWeight: 700 }}>{t("settings.theme")}</div>
            <Card style={{ padding: 16, display: "flex", alignItems: "center", gap: 12 }}>
              <span style={{ flex: 1, fontSize: 13 }}>{t("settings.theme.desc")}</span>
              <Button variant="primary" onClick={onToggleTheme}>
                {dark ? t("settings.theme.toggleLight") : t("settings.theme.toggleDark")}
              </Button>
            </Card>
            <p style={{ margin: 0, fontSize: 12, color: "var(--text-3)", lineHeight: 1.5 }}>{t("settings.contextDir.note")}</p>
          </>
        )}
      </div>
    </div>
  );
}

function fieldLabel(t: (k: MsgKey) => string, key?: string): string {
  if (!key) return "";
  const mk = FIELD_KEYS[key];
  return mk ? t(mk) : key;
}

function FieldBadge({ t, field }: { t: (k: MsgKey) => string; field?: GatewayField }) {
  if (!field) return null;
  if (isFieldEnvOwned(field)) return <Badge tone="neutral">{t("settings.envOwned")}</Badge>;
  if (!isFieldEditable(field)) return <Badge tone="neutral">{t("settings.readonly")}</Badge>;
  return null;
}

function GatewayCard({ title, icon, children }: { title: string; icon: IconName; children: ReactNode }) {
  return (
    <Card>
      <CardHeader icon={icon} title={title} />
      <div style={{ display: "flex", flexDirection: "column" }}>{children}</div>
    </Card>
  );
}

function ReadRow({ t, field, boolean }: { t: (k: MsgKey) => string; field?: GatewayField; boolean?: boolean }) {
  const raw = boolean ? (field?.value === true ? "yes" : "no") : fieldValue(field) || "—";
  const shown = boolean ? t(raw === "yes" ? "common.yes" : "common.no") : raw;
  return (
    <div style={{ display: "flex", gap: 8, alignItems: "center", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 13 }}>
      <span style={{ flex: 1.2, color: "var(--text-2)" }}>{fieldLabel(t, field?.key)}</span>
      <span style={{ flex: 1.4, fontWeight: 600 }}>{shown}</span>
      <span style={{ width: 110, textAlign: "right" }}>
        <FieldBadge t={t} field={field} />
      </span>
    </div>
  );
}

function EditRow({
  t,
  field,
  value,
  onChange,
  options,
  number,
}: {
  t: (k: MsgKey) => string;
  field?: GatewayField;
  value: string;
  onChange: (v: string) => void;
  options?: string[];
  number?: boolean;
}) {
  const locked = !isFieldEditable(field);
  return (
    <div style={{ display: "flex", gap: 8, alignItems: "center", padding: "8px 16px", borderBottom: "1px solid var(--border-soft)", fontSize: 13 }}>
      <span style={{ flex: 1.2, color: "var(--text-2)" }}>{fieldLabel(t, field?.key)}</span>
      <span style={{ flex: 1.4 }}>
        {options ? (
          <select className="z-field" value={value} disabled={locked} onChange={(e) => onChange(e.target.value)} aria-label={fieldLabel(t, field?.key)} style={{ width: "100%" }}>
            {options.map((o) => (
              <option key={o || "empty"} value={o}>
                {o || "—"}
              </option>
            ))}
          </select>
        ) : (
          <input
            className="z-field"
            type={number ? "number" : "text"}
            min={number ? 0 : undefined}
            value={value}
            disabled={locked}
            onChange={(e) => onChange(e.target.value)}
            aria-label={fieldLabel(t, field?.key)}
            style={{ width: "100%" }}
          />
        )}
      </span>
      <span style={{ width: 110, textAlign: "right" }}>
        <FieldBadge t={t} field={field} />
      </span>
    </div>
  );
}

function Row({ children, head }: { children: ReactNode; head?: boolean }) {
  return (
    <div
      style={{
        display: "flex",
        alignItems: "center",
        padding: head ? "8px 16px" : "11px 16px",
        fontSize: head ? 10 : 12.5,
        fontWeight: head ? 600 : 400,
        letterSpacing: head ? ".4px" : undefined,
        color: head ? "var(--text-3)" : undefined,
        borderBottom: "1px solid var(--border-soft)",
        gap: 8,
      }}
    >
      {children}
    </div>
  );
}
