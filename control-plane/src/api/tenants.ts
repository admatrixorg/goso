import { jsonFetch } from "./client";
import { asPublic, asPublicContext, type Tenant, type TenantContext, type TenantList } from "./tenants-ops";

export type { Tenant, TenantContext, TenantList, TenantMember } from "./tenants-ops";

async function one(path: string, init?: RequestInit): Promise<Tenant> {
  const row = await jsonFetch<Tenant>(path, init);
  const pub = asPublic([row])[0];
  if (!pub) throw new Error("secret-shaped payload");
  return pub;
}

export const tenantsApi = {
  list: async (q?: string): Promise<TenantList> => {
    const qs = q && q.trim() ? `?q=${encodeURIComponent(q.trim())}` : "";
    const j = await jsonFetch<TenantList>(`/api/tenants${qs}`);
    return {
      tenants: asPublic(j.tenants),
      current: asPublicContext(j.current),
      master: asPublicContext(j.master),
      multi_tenant: Boolean(j.multi_tenant),
    };
  },
  get: (id: string) => one(`/api/tenants/${encodeURIComponent(id)}`),
  create: (slug: string, name: string) =>
    one("/api/tenants", { method: "POST", body: JSON.stringify({ slug, name }) }),
  setStatus: (id: string, status: "active" | "deactivated", confirm?: string) =>
    one(`/api/tenants/${encodeURIComponent(id)}/status`, {
      method: "POST",
      body: JSON.stringify({ status, confirm }),
    }),
  addMember: (id: string, subject: string, role: string) =>
    one(`/api/tenants/${encodeURIComponent(id)}/members`, {
      method: "POST",
      body: JSON.stringify({ subject, role }),
    }),
  setMemberRole: (id: string, mid: string, role: string) =>
    one(`/api/tenants/${encodeURIComponent(id)}/members/${encodeURIComponent(mid)}`, {
      method: "PATCH",
      body: JSON.stringify({ role }),
    }),
  removeMember: (id: string, mid: string, confirm: string) =>
    one(`/api/tenants/${encodeURIComponent(id)}/members/${encodeURIComponent(mid)}`, {
      method: "DELETE",
      body: JSON.stringify({ confirm }),
    }),
};
