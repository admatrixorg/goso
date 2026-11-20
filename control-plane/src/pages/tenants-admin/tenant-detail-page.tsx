import { useState, useMemo, useRef } from "react";
import { useParams, useNavigate } from "react-router";
import { useTranslation } from "react-i18next";
import { ArrowLeft, Plus, RefreshCw, Users, Trash2, Calendar, Hash, Shield, Copy, Check, KeyRound } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { Badge } from "@/components/ui/badge";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { PageHeader } from "@/components/shared/page-header";
import { EmptyState } from "@/components/shared/empty-state";
import { TableSkeleton } from "@/components/shared/loading-skeleton";
import { UserPickerCombobox } from "@/components/shared/user-picker-combobox";
import { useContactResolver } from "@/hooks/use-contact-resolver";
import { formatUserLabel } from "@/lib/format-user-label";
import { useDeferredLoading } from "@/hooks/use-deferred-loading";
import { useMinLoading } from "@/hooks/use-min-loading";
import { useHttp } from "@/hooks/use-ws";
import { ApiError } from "@/api/errors";
import { toast } from "@/stores/use-toast-store";
import { useTenantDetail } from "./hooks/use-tenant-detail";
import { useAccountDirectory } from "./hooks/use-account-directory";
import { selectableAccounts } from "./account-directory";
import { ROUTES } from "@/lib/constants";

const TENANT_ROLES = ["owner", "admin", "operator", "member", "viewer"] as const;

const MIN_PASSWORD_LENGTH = 8;

const ROLE_KEYS: Record<string, string> = {
  owner: "roleOwner", admin: "roleAdmin", operator: "roleOperator",
  member: "roleMember", viewer: "roleViewer",
};

const ROLE_COLORS: Record<string, string> = {
  owner: "bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-300",
  admin: "bg-orange-100 text-orange-800 dark:bg-orange-900/30 dark:text-orange-300",
  operator: "bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300",
  member: "bg-muted text-muted-foreground",
  viewer: "bg-muted text-muted-foreground",
};

/** Response of POST /v1/users (create user + tenant membership). */
interface CreateUserResponse {
  user?: { id?: string; email?: string };
  membership?: { tenant_id?: string; role?: string };
  one_time_password?: string;
}

export function TenantDetailPage() {
  const { id = "" } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { t } = useTranslation("tenants");
  const { t: tc } = useTranslation("common");
  const http = useHttp();

  const { tenant, tenantLoading, users, usersLoading, usersRefreshing, refreshUsers, addUser, removeUser } =
    useTenantDetail(id);

  const spinning = useMinLoading(usersRefreshing);
  const showSkeleton = useDeferredLoading(usersLoading && users.length === 0);

  // Resolve user IDs to display names via contacts
  const userIds = useMemo(() => users.map((u) => u.user_id), [users]);
  const { resolve } = useContactResolver(userIds);

  const [addOpen, setAddOpen] = useState(false);
  const [mode, setMode] = useState<"existing" | "create">("existing");
  const [userId, setUserId] = useState("");
  // Tim tai khoan trong danh ba toan he thong (moi tenant), va loi thoat cho
  // thanh vien kieu cu — dinh danh chuoi nhu "trung-admin" khong nam trong
  // bang users nen khong the hien trong danh ba.
  const [accountQuery, setAccountQuery] = useState("");
  const [manualId, setManualId] = useState(false);
  const [role, setRole] = useState("member");
  const [adding, setAdding] = useState(false);
  const [removeTarget, setRemoveTarget] = useState<string | null>(null);
  const [removing, setRemoving] = useState(false);

  // Tab "create new user"
  const [email, setEmail] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [manualPassword, setManualPassword] = useState(false);
  const [password, setPassword] = useState("");
  const [creating, setCreating] = useState(false);
  const [createdCredential, setCreatedCredential] = useState<{ email: string; password: string } | null>(null);
  const [copied, setCopied] = useState(false);
  const passwordOutRef = useRef<HTMLInputElement>(null);

  const passwordTooShort = manualPassword && password.length > 0 && password.length < MIN_PASSWORD_LENGTH;

  // Bat luon: danh ba con dung de dich UUID -> email o danh sach thanh vien,
  // khong chi cho o chon nguoi. Ket qua duoc cache 30s nen khong ton them.
  const directory = useAccountDirectory(true);

  /**
   * Nhan cua mot thanh vien.
   *
   * tenant_users.display_name chi duoc dien khi user duoc TAO trong tenant do;
   * them mot tai khoan co san vao tenant khac thi cot ay rong (store.AddUser
   * khong ghi display_name), va man hinh roi ve UUID tho — khong ai quan tri
   * noi. Nen tra cuu email tu danh ba lam nhan chinh.
   */
  const emailById = useMemo(
    () => new Map(directory.accounts.map((a) => [a.id, a.email])),
    [directory.accounts],
  );
  const memberLabel = (userId: string, displayName?: string): string =>
    displayName || emailById.get(userId) || formatUserLabel(userId, resolve);
  const candidates = useMemo(
    () => selectableAccounts(directory.accounts, id, accountQuery),
    [directory.accounts, id, accountQuery],
  );
  const canCreate =
    email.trim().length > 0 && (!manualPassword || password.length >= MIN_PASSWORD_LENGTH);

  const resetCreateForm = () => {
    setEmail("");
    setDisplayName("");
    setManualPassword(false);
    setPassword("");
  };

  const handleAdd = async () => {
    if (!userId.trim()) return;
    setAdding(true);
    try {
      await addUser(userId.trim(), role);
      setAddOpen(false);
      setUserId("");
      setRole("member");
      setAccountQuery("");
      setManualId(false);
    } finally {
      setAdding(false);
    }
  };

  const handleCreate = async () => {
    const mail = email.trim();
    if (!mail) {
      toast.error(t("createUser"), t("emailRequired"));
      return;
    }
    if (manualPassword && password.length < MIN_PASSWORD_LENGTH) {
      toast.error(t("createUser"), t("passwordTooShort"));
      return;
    }
    setCreating(true);
    try {
      const res = await http.post<CreateUserResponse>("/v1/users", {
        email: mail,
        tenant_id: tenant?.id ?? id,
        role,
        ...(displayName.trim() ? { display_name: displayName.trim() } : {}),
        ...(manualPassword ? { password } : {}),
      });
      setAddOpen(false);
      resetCreateForm();
      setRole("member");
      setCopied(false);
      setCreatedCredential({
        email: res?.user?.email ?? mail,
        password: res?.one_time_password ?? (manualPassword ? password : ""),
      });
      await refreshUsers();
    } catch (err) {
      const isDuplicate =
        err instanceof ApiError && (err.code === "ALREADY_EXISTS" || /exist|tồn tại/i.test(err.message));
      toast.error(
        t("createUser"),
        isDuplicate ? t("emailExists") : err instanceof Error ? err.message : "",
      );
    } finally {
      setCreating(false);
    }
  };

  const handleCopyPassword = async () => {
    if (!createdCredential?.password) return;
    try {
      await navigator.clipboard.writeText(createdCredential.password);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // Clipboard blocked (non-secure origin / permission): select the text so the user can copy manually.
      passwordOutRef.current?.focus();
      passwordOutRef.current?.select();
    }
  };

  const handleRemove = async () => {
    if (!removeTarget) return;
    setRemoving(true);
    try {
      await removeUser(removeTarget);
      setRemoveTarget(null);
    } finally {
      setRemoving(false);
    }
  };

  if (tenantLoading) {
    return <div className="p-4 sm:p-6 pb-10"><TableSkeleton rows={3} /></div>;
  }

  return (
    <div className="p-4 sm:p-6 space-y-6">
      <PageHeader
        title={tenant?.name ?? t("detail")}
        description=""
        actions={
          <Button variant="outline" size="sm" onClick={() => navigate(ROUTES.TENANTS)} className="gap-1">
            <ArrowLeft className="h-3.5 w-3.5" /> {t("back")}
          </Button>
        }
      />

      {/* Tenant Info Card */}
      {tenant && (
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
          <InfoCard icon={Hash} label={t("slug")} value={tenant.slug} mono />
          <InfoCard icon={Shield} label={t("status")}>
            <Badge variant={tenant.status === "active" ? "default" : tenant.status === "suspended" ? "destructive" : "secondary"}>
              {t(tenant.status) || tenant.status}
            </Badge>
          </InfoCard>
          <InfoCard icon={Calendar} label={t("created")} value={new Date(tenant.created_at).toLocaleDateString()} />
        </div>
      )}

      {/* User Management */}
      <div>
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-base font-semibold flex items-center gap-2">
            <Users className="h-4 w-4 text-muted-foreground" />
            {t("userManagement")}
            {users.length > 0 && (
              <span className="text-xs font-normal text-muted-foreground">({users.length})</span>
            )}
          </h2>
          <div className="flex gap-2">
            <Button size="sm" onClick={() => setAddOpen(true)} className="gap-1">
              <Plus className="h-3.5 w-3.5" /> {t("addUser")}
            </Button>
            <Button variant="outline" size="sm" onClick={refreshUsers} disabled={spinning} className="gap-1">
              <RefreshCw className={spinning ? "animate-spin h-3.5 w-3.5" : "h-3.5 w-3.5"} />
            </Button>
          </div>
        </div>

        {showSkeleton ? (
          <TableSkeleton rows={4} />
        ) : users.length === 0 ? (
          <EmptyState icon={Users} title={t("noUsers")} description="" />
        ) : (
          <div className="grid gap-2">
            {users.map((u) => (
              <div key={u.user_id} className="flex items-center justify-between rounded-lg border px-4 py-3 hover:bg-muted/30 transition-colors">
                <div className="flex items-center gap-3 min-w-0">
                  <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-muted text-xs font-medium uppercase">
                    {memberLabel(u.user_id, u.display_name).charAt(0)}
                  </div>
                  <div className="min-w-0">
                    <p className="text-sm font-medium truncate">{memberLabel(u.user_id, u.display_name)}</p>
                    <p className="text-xs text-muted-foreground truncate">
                      {/* Email lam dong phu de phan biet hai nguoi trung ten. */}
                      {emailById.get(u.user_id) && emailById.get(u.user_id) !== memberLabel(u.user_id, u.display_name)
                        ? emailById.get(u.user_id) + " · "
                        : ""}
                      {new Date(u.created_at).toLocaleDateString()}
                    </p>
                  </div>
                </div>
                <div className="flex items-center gap-2 shrink-0">
                  <span className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${ROLE_COLORS[u.role] || ROLE_COLORS.member}`}>
                    {t(ROLE_KEYS[u.role] ?? u.role)}
                  </span>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-8 w-8 p-0 text-muted-foreground hover:text-destructive"
                    onClick={() => setRemoveTarget(u.user_id)}
                    title={t("removeUser")}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </Button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Add User Dialog */}
      <Dialog open={addOpen} onOpenChange={setAddOpen}>
        <DialogContent className="max-sm:inset-0 max-sm:translate-x-0 max-sm:translate-y-0 sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t("addUserTitle")}</DialogTitle>
            <DialogDescription>{t("description")}</DialogDescription>
          </DialogHeader>

          <Tabs value={mode} onValueChange={(v) => setMode(v as "existing" | "create")}>
            <TabsList className="w-full">
              <TabsTrigger value="existing">{t("tabExisting")}</TabsTrigger>
              <TabsTrigger value="create">{t("tabCreate")}</TabsTrigger>
            </TabsList>

            <TabsContent value="existing">
              <div className="space-y-4 py-2">
                {manualId ? (
                  <div className="space-y-1.5">
                    <Label>{t("userId")}</Label>
                    <UserPickerCombobox
                      value={userId}
                      onChange={setUserId}
                      placeholder="user-id"
                      source="tenant_user"
                      allowCustom={true}
                    />
                    <button
                      type="button"
                      className="text-xs text-primary hover:underline"
                      onClick={() => { setManualId(false); setUserId(""); }}
                    >
                      {t("backToAccountSearch")}
                    </button>
                  </div>
                ) : (
                  <div className="space-y-1.5">
                    <Label>{t("pickAccount")}</Label>
                    <Input
                      value={accountQuery}
                      onChange={(e) => setAccountQuery(e.target.value)}
                      placeholder={t("searchAccountPlaceholder")}
                      className="text-base md:text-sm"
                    />

                    <div className="max-h-56 overflow-y-auto rounded-md border">
                      {directory.loading ? (
                        <p className="p-3 text-sm text-muted-foreground">{tc("loading")}</p>
                      ) : candidates.length === 0 ? (
                        <p className="p-3 text-sm text-muted-foreground">{t("noAccountFound")}</p>
                      ) : (
                        candidates.map((a) => (
                          <button
                            key={a.id}
                            type="button"
                            onClick={() => setUserId(a.id)}
                            className={
                              "flex w-full flex-col items-start gap-0.5 border-b px-3 py-2 text-left last:border-b-0 hover:bg-muted/60 "
                              + (userId === a.id ? "bg-primary/10" : "")
                            }
                          >
                            <span className="text-sm font-medium">{a.email}</span>
                            <span className="text-xs text-muted-foreground">
                              {a.displayName ? a.displayName + " · " : ""}
                              {a.tenantIds.length
                                ? t("currentlyIn", {
                                    tenants: a.tenantIds
                                      .map((tid) => directory.tenantNameById.get(tid) ?? tid)
                                      .join(", "),
                                  })
                                : t("noTenantYet")}
                            </span>
                          </button>
                        ))
                      )}
                    </div>

                    <button
                      type="button"
                      className="text-xs text-primary hover:underline"
                      onClick={() => { setManualId(true); setUserId(""); }}
                    >
                      {t("enterUserIdManually")}
                    </button>
                  </div>
                )}
                <div className="space-y-1.5">
                  <Label>{t("selectRole")}</Label>
                  <Select value={role} onValueChange={setRole}>
                    <SelectTrigger className="text-base md:text-sm"><SelectValue /></SelectTrigger>
                    <SelectContent>
                      {TENANT_ROLES.map((r) => (
                        <SelectItem key={r} value={r}>{t(ROLE_KEYS[r] ?? r)}</SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              </div>
            </TabsContent>

            <TabsContent value="create">
              <div className="space-y-4 py-2">
                <div className="space-y-1.5">
                  <Label htmlFor="new-user-email">{t("emailLabel")}</Label>
                  <Input
                    id="new-user-email"
                    type="email"
                    autoComplete="off"
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                    placeholder="user@company.vn"
                    className="text-base md:text-sm"
                  />
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="new-user-name">{t("displayNameLabel")}</Label>
                  <Input
                    id="new-user-name"
                    value={displayName}
                    onChange={(e) => setDisplayName(e.target.value)}
                    className="text-base md:text-sm"
                  />
                </div>
                <div className="space-y-1.5">
                  <Label>{t("selectRole")}</Label>
                  <Select value={role} onValueChange={setRole}>
                    <SelectTrigger className="text-base md:text-sm"><SelectValue /></SelectTrigger>
                    <SelectContent>
                      {TENANT_ROLES.map((r) => (
                        <SelectItem key={r} value={r}>{t(ROLE_KEYS[r] ?? r)}</SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <div className="flex items-center justify-between gap-3 rounded-lg border px-3 py-2">
                  <Label htmlFor="manual-password" className="text-sm font-normal">
                    {t("manualPasswordLabel")}
                  </Label>
                  <Switch
                    id="manual-password"
                    checked={manualPassword}
                    onCheckedChange={(v) => {
                      setManualPassword(v);
                      if (!v) setPassword("");
                    }}
                  />
                </div>
                {manualPassword && (
                  <div className="space-y-1.5">
                    <Label htmlFor="new-user-password">{t("passwordLabel")}</Label>
                    <Input
                      id="new-user-password"
                      type="password"
                      autoComplete="new-password"
                      value={password}
                      onChange={(e) => setPassword(e.target.value)}
                      aria-invalid={passwordTooShort || undefined}
                      className="text-base md:text-sm"
                    />
                    {passwordTooShort && (
                      <p className="text-xs text-destructive">{t("passwordTooShort")}</p>
                    )}
                  </div>
                )}
              </div>
            </TabsContent>
          </Tabs>

          <DialogFooter>
            <Button variant="outline" onClick={() => setAddOpen(false)} disabled={adding || creating}>
              {tc("cancel")}
            </Button>
            {mode === "existing" ? (
              <Button onClick={handleAdd} disabled={adding || !userId.trim()}>{t("addUser")}</Button>
            ) : (
              <Button onClick={handleCreate} disabled={creating || !canCreate}>{t("createUser")}</Button>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Created credential Dialog — password shown once */}
      <Dialog open={!!createdCredential} onOpenChange={(o) => { if (!o) setCreatedCredential(null); }}>
        <DialogContent className="max-sm:inset-0 max-sm:translate-x-0 max-sm:translate-y-0 sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <KeyRound className="h-4 w-4" /> {t("createdTitle")}
            </DialogTitle>
            <DialogDescription>{t("createdWarning")}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div className="space-y-1.5">
              <Label>{t("emailLabel")}</Label>
              <Input readOnly value={createdCredential?.email ?? ""} className="font-mono text-base md:text-sm" />
            </div>
            <div className="space-y-1.5">
              <Label>{t("passwordLabel")}</Label>
              <div className="flex gap-2">
                <Input
                  ref={passwordOutRef}
                  readOnly
                  value={createdCredential?.password ?? ""}
                  onFocus={(e) => e.currentTarget.select()}
                  className="font-mono text-base md:text-sm"
                />
                <Button variant="outline" onClick={handleCopyPassword} className="gap-1 shrink-0">
                  {copied ? <Check className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}
                  {copied ? t("copied") : t("copyPassword")}
                </Button>
              </div>
            </div>
          </div>
          <DialogFooter>
            <Button onClick={() => setCreatedCredential(null)}>{tc("close")}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={!!removeTarget}
        onOpenChange={(o) => { if (!o) setRemoveTarget(null); }}
        title={t("removeUser")}
        description={t("confirmRemoveUser")}
        confirmLabel={t("removeUser")}
        variant="destructive"
        onConfirm={handleRemove}
        loading={removing}
      />
    </div>
  );
}

function InfoCard({ icon: Icon, label, value, mono, children }: {
  icon: React.ComponentType<{ className?: string }>;
  label: string;
  value?: string;
  mono?: boolean;
  children?: React.ReactNode;
}) {
  return (
    <div className="rounded-lg border p-3 flex items-start gap-3">
      <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-muted">
        <Icon className="h-4 w-4 text-muted-foreground" />
      </div>
      <div className="min-w-0">
        <p className="text-xs text-muted-foreground">{label}</p>
        {children ?? (
          <p className={`text-sm font-medium truncate ${mono ? "font-mono" : ""}`}>{value}</p>
        )}
      </div>
    </div>
  );
}
