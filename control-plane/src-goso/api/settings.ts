import { jsonFetch } from "./client";
import { asList, crmOrgId, crmRequest } from "./crm";
import type { GatewayConfig, GatewayPatch } from "./settings-ops";

export type { GatewayConfig, GatewayField, GatewayPatch } from "./settings-ops";

export type SettingsUser = { id: string; orgId: string; name: string; email: string; roleId: string; active: boolean };
export type SettingsRole = { id: string; orgId: string; name: string; flags: Record<string, unknown> };
export type SettingsNick = { id: string; orgId: string; displayName: string };
export type SettingsQuota = { orgId: string; dailySendCap: number };
export type SettingsTemplate = { id: string; orgId: string; name: string; body: string };
export type SettingsAccount = { orgId: string; displayName: string };
export type Developing = { status: string };

function org(orgId?: string): string {
  return (orgId ?? crmOrgId()).trim();
}

function str(v: unknown): string {
  return typeof v === "string" ? v : "";
}

function flagsOf(v: unknown): Record<string, unknown> {
  if (v && typeof v === "object" && !Array.isArray(v)) return v as Record<string, unknown>;
  if (typeof v === "string") {
    try {
      const p = JSON.parse(v) as unknown;
      if (p && typeof p === "object" && !Array.isArray(p)) return p as Record<string, unknown>;
    } catch {
      return {};
    }
  }
  return {};
}

function userOf(r: Record<string, unknown>): SettingsUser {
  return {
    id: str(r.id),
    orgId: str(r.orgId),
    name: str(r.name),
    email: str(r.email),
    roleId: str(r.roleId),
    active: r.active !== false,
  };
}

export const settingsApi = {
  listUsers: async (orgId?: string) => asList<Record<string, unknown>>(await crmRequest("/api/settings/users", org(orgId))).map(userOf),
  createUser: (body: { name: string; email?: string; roleId?: string; active?: boolean }, orgId?: string) =>
    crmRequest<SettingsUser>("/api/settings/users", org(orgId), { method: "POST", body: JSON.stringify(body) }).then((r) =>
      userOf(r as unknown as Record<string, unknown>),
    ),
  patchUser: (id: string, body: { name?: string; email?: string; roleId?: string; active?: boolean }, orgId?: string) =>
    crmRequest<SettingsUser>(`/api/settings/users/${encodeURIComponent(id)}`, org(orgId), {
      method: "PATCH",
      body: JSON.stringify(body),
    }).then((r) => userOf(r as unknown as Record<string, unknown>)),
  deleteUser: (id: string, orgId?: string) =>
    crmRequest<void>(`/api/settings/users/${encodeURIComponent(id)}`, org(orgId), { method: "DELETE" }),

  listRoles: async (orgId?: string) =>
    asList<Record<string, unknown>>(await crmRequest("/api/settings/roles", org(orgId))).map((r) => ({
      id: str(r.id),
      orgId: str(r.orgId),
      name: str(r.name),
      flags: flagsOf(r.flags),
    })),
  createRole: (body: { name: string; flags?: Record<string, unknown> }, orgId?: string) =>
    crmRequest<SettingsRole>("/api/settings/roles", org(orgId), { method: "POST", body: JSON.stringify(body) }),

  listNicks: async (orgId?: string) =>
    asList<Record<string, unknown>>(await crmRequest("/api/settings/nicks", org(orgId))).map((r) => ({
      id: str(r.id),
      orgId: str(r.orgId),
      displayName: str(r.displayName),
    })),
  createNick: (body: { displayName: string }, orgId?: string) =>
    crmRequest<SettingsNick>("/api/settings/nicks", org(orgId), { method: "POST", body: JSON.stringify(body) }),

  getQuota: (orgId?: string) => crmRequest<SettingsQuota>("/api/settings/quotas", org(orgId)),
  putQuota: (body: { dailySendCap: number }, orgId?: string) =>
    crmRequest<SettingsQuota>("/api/settings/quotas", org(orgId), { method: "PUT", body: JSON.stringify(body) }),

  listTemplates: async (orgId?: string) =>
    asList<Record<string, unknown>>(await crmRequest("/api/settings/templates", org(orgId))).map((r) => ({
      id: str(r.id),
      orgId: str(r.orgId),
      name: str(r.name),
      body: str(r.body),
    })),
  createTemplate: (body: { name: string; body: string }, orgId?: string) =>
    crmRequest<SettingsTemplate>("/api/settings/templates", org(orgId), { method: "POST", body: JSON.stringify(body) }),
  deleteTemplate: (id: string, orgId?: string) =>
    crmRequest<void>(`/api/settings/templates/${encodeURIComponent(id)}`, org(orgId), { method: "DELETE" }),

  getAccount: (orgId?: string) => crmRequest<SettingsAccount>("/api/settings/account", org(orgId)),
  putAccount: (body: { displayName: string }, orgId?: string) =>
    crmRequest<SettingsAccount>("/api/settings/account", org(orgId), { method: "PUT", body: JSON.stringify(body) }),

  billing: (orgId?: string) => crmRequest<Developing>("/api/settings/billing", org(orgId)),
  placeholders: (orgId?: string) => crmRequest<Developing>("/api/settings/placeholders", org(orgId)),

  getGateway: () => jsonFetch<GatewayConfig>("/api/config"),
  putGateway: (body: GatewayPatch) =>
    jsonFetch<GatewayConfig>("/api/config", { method: "PUT", body: JSON.stringify(body) }),
};
