import { useEffect, useState, type ReactNode } from "react";
import { backupApi, type BackupFile } from "../api/backup";
import { crmOrgId } from "../api/crm";
import {
  settingsApi,
  type SettingsAccount,
  type SettingsNick,
  type SettingsQuota,
  type SettingsRole,
  type SettingsTemplate,
  type SettingsUser,
} from "../api/settings";
import { useI18n } from "../i18n";
import { Badge } from "../ui/Badge";
import { Button } from "../ui/Button";
import { Card, CardHeader, TableScroll } from "../ui/Card";
import { EmptyState } from "../ui/EmptyState";
import { Icon, type IconName } from "../ui/Icon";

type PageId = "account" | "users" | "roles" | "nicks" | "quotas" | "templates" | "billing" | "backup" | "theme";

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
  const [backups, setBackups] = useState<BackupFile[]>([]);
  const [backupNote, setBackupNote] = useState("");
  const [backupBusy, setBackupBusy] = useState(false);

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
        { id: "backup", label: t("settings.backup"), ic: "download" },
        { id: "theme", label: t("settings.theme"), ic: "sun" },
      ],
    },
  ];

  async function load() {
    const id = org.trim() || crmOrgId();
    setErr("");
    setDeveloping("");
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
      } else if (page === "billing") {
        const d = await settingsApi.billing(id);
        setDeveloping(d.status || "developing");
      } else if (page === "backup") {
        const list = await backupApi.list();
        setBackups(list.files || []);
      }
    } catch (e) {
      setErr(String(e));
    }
  }

  useEffect(() => {
    if (page === "theme") return;
    void load();
  }, [page, org]);

  async function run(fn: () => Promise<unknown>) {
    try {
      setErr("");
      await fn();
      await load();
    } catch (e) {
      setErr(String(e));
    }
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
        {page !== "theme" && page !== "backup" ? (
          <div style={{ display: "flex", gap: 8, alignItems: "center", flexWrap: "wrap" }}>
            <span style={{ fontSize: 12, color: "var(--text-3)" }}>{t("common.org")}</span>
            <input className="z-field" style={{ minWidth: 0, flex: 1 }} value={org} onChange={(e) => setOrg(e.target.value)} aria-label="CRM org id" />
            <Button icon="refresh" iconGesture onClick={() => void load()}>
              {t("common.refresh")}
            </Button>
          </div>
        ) : null}
        {err ? <p style={{ color: "var(--red)", fontSize: 12.5, margin: 0 }}>{err}</p> : null}

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
              <Button variant="primary" onClick={() => void run(() => settingsApi.putAccount({ displayName: displayName.trim() }, org))}>
                {t("common.save")}
              </Button>
            </Card>
            {account?.orgId ? (
              <div style={{ fontSize: 12, color: "var(--text-3)" }}>
                org {account.orgId}
                {account.displayName ? ` · ${account.displayName}` : ""}
              </div>
            ) : null}
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
                    if (!userName.trim()) return;
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
                    if (!roleName.trim()) return;
                    let flags: Record<string, unknown> = {};
                    try {
                      flags = JSON.parse(roleFlags || "{}") as Record<string, unknown>;
                    } catch {
                      setErr("flags must be a JSON object");
                      return;
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
                    if (!nickName.trim()) return;
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
                onClick={() => void run(() => settingsApi.putQuota({ dailySendCap: Number.parseInt(cap, 10) || 0 }, org))}
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
                    if (!tplName.trim()) return;
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

        {page === "backup" && (
          <>
            <div style={{ fontSize: 21, fontWeight: 700 }}>{t("settings.backup")}</div>
            <div style={{ fontSize: 12.5, color: "var(--text-3)" }}>{t("settings.backup.desc")}</div>
            <div style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "center" }}>
              <Button
                variant="primary"
                icon="plus"
                disabled={backupBusy}
                onClick={() =>
                  void run(async () => {
                    if (backupBusy) return;
                    setBackupBusy(true);
                    setBackupNote("");
                    try {
                      const snap = await backupApi.create();
                      setBackupNote(t("settings.backup.created", { file: snap.file }));
                    } finally {
                      setBackupBusy(false);
                    }
                  })
                }
              >
                {t("settings.backup.create")}
              </Button>
              <Button icon="refresh" iconGesture onClick={() => void load()}>
                {t("common.refresh")}
              </Button>
            </div>
            {backupNote ? <p style={{ color: "var(--text-2)", fontSize: 12.5, margin: 0 }}>{backupNote}</p> : null}
            <Card>
              <CardHeader icon="download" title={t("settings.backup")} meta={String(backups.length)} />
              <TableScroll>
                <Row head>
                  <span style={{ flex: 2 }}>{t("settings.col.file")}</span>
                  <span style={{ flex: 0.8 }}>{t("settings.col.bytes")}</span>
                  <span style={{ flex: 1.2 }}>{t("settings.col.mtime")}</span>
                  <span style={{ flex: 0.8 }}>{t("settings.col.integrity")}</span>
                  <span style={{ width: 140 }} />
                </Row>
                {backups.map((b) => (
                  <Row key={b.file}>
                    <span style={{ flex: 2, fontWeight: 600, fontFamily: "var(--font-mono, ui-monospace)", fontSize: 12 }}>{b.file}</span>
                    <span style={{ flex: 0.8, color: "var(--text-2)", fontVariantNumeric: "tabular-nums" }}>{b.bytes}</span>
                    <span style={{ flex: 1.2, color: "var(--text-3)", fontSize: 12 }}>{b.mtime || "—"}</span>
                    <span style={{ flex: 0.8 }}>
                      <Badge tone={b.integrity === "ok" ? "positive" : "critical"}>
                        {b.integrity === "ok" ? t("settings.backup.ok") : t("settings.backup.fail")}
                      </Badge>
                    </span>
                    <span style={{ width: 140, textAlign: "right" }}>
                      <Button
                        variant="quiet"
                        style={{ padding: "4px 8px" }}
                        onClick={() =>
                          void run(async () => {
                            if (!window.confirm(t("settings.backup.confirmRestore"))) return;
                            setBackupNote("");
                            const r = await backupApi.restore(b.file);
                            setBackupNote(t("settings.backup.restored", { file: r.file || b.file }));
                          })
                        }
                      >
                        {t("settings.backup.restore")}
                      </Button>
                    </span>
                  </Row>
                ))}
                {backups.length === 0 ? <EmptyState>{t("settings.backup.empty")}</EmptyState> : null}
              </TableScroll>
            </Card>
            <p style={{ margin: 0, fontSize: 12, color: "var(--text-3)", lineHeight: 1.5 }}>{t("settings.backup.applyHint")}</p>
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
