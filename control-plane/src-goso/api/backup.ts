import { jsonFetch, TENANT_STORAGE_KEY } from "./client";
import { gatewayFetchInit } from "./gateway-http";
import {
  asPublicFile,
  asPublicList,
  asPublicPlan,
  asPublicPreflight,
  asPublicS3,
  type BackupDest,
  type BackupFile,
  type BackupList,
  type BackupScope,
  type Preflight,
  type RestorePlan,
  type S3Status,
  type S3Write,
} from "./backup-ops";

export type { BackupDest, BackupFile, BackupList, BackupScope, Preflight, RestorePlan, S3Status, S3Write } from "./backup-ops";

function gatewayBase(): string {
  const raw = (import.meta.env.VITE_GATEWAY_URL as string) || "";
  return raw.replace(/\/$/, "");
}

function authHeaders(): Record<string, string> {
  const t = (import.meta.env.VITE_GOSO_ADMIN_TOKEN as string) || localStorage.getItem("goso_token") || "";
  const h: Record<string, string> = {};
  if (t) h.Authorization = `Bearer ${t}`;
  try {
    const tenant = (localStorage.getItem(TENANT_STORAGE_KEY) || "").trim();
    if (tenant) h["X-Goso-Tenant"] = tenant;
  } catch {
    /* private mode */
  }
  return h;
}

export const backupApi = {
  list: async (scope?: BackupScope): Promise<BackupList> => {
    const q = scope ? `?scope=${encodeURIComponent(scope)}` : "";
    return asPublicList(await jsonFetch<BackupList>(`/api/system/backup${q}`));
  },
  preflight: async (): Promise<Preflight> => asPublicPreflight(await jsonFetch<Preflight>("/api/system/backup/preflight")),
  create: async (scope: BackupScope = "system", tenant = "", destination: BackupDest = "local"): Promise<BackupFile> => {
    const row = asPublicFile(
      await jsonFetch<BackupFile>("/api/system/backup", {
        method: "POST",
        body: JSON.stringify({ scope, tenant, destination }),
      }),
    );
    if (!row) throw new Error("secret-shaped payload");
    return row;
  },
  restore: async (file: string, confirm = "") =>
    jsonFetch<{ file: string; integrity: string; applied: boolean; credentials_excluded?: boolean }>("/api/system/restore", {
      method: "POST",
      body: JSON.stringify(confirm ? { file, confirm } : { file }),
    }),
  validate: async (file: string) => jsonFetch<{ valid: boolean; manifest: unknown }>("/api/system/backup/validate", { method: "POST", body: JSON.stringify({ file }) }),
  plan: async (file: string): Promise<RestorePlan> => {
    const row = asPublicPlan(await jsonFetch<RestorePlan>("/api/system/restore/plan", { method: "POST", body: JSON.stringify({ file }) }));
    if (!row) throw new Error("secret-shaped payload");
    return row;
  },
  download: async (file: string): Promise<Blob> => {
    const res = await fetch(
      `${gatewayBase()}/api/system/backup/download?file=${encodeURIComponent(file)}`,
      gatewayFetchInit({ headers: authHeaders() }),
    );
    if (!res.ok) {
      const text = await res.text();
      throw new Error(`${res.status} ${text}`);
    }
    return res.blob();
  },
  s3: async (): Promise<S3Status> => {
    const row = asPublicS3(await jsonFetch<S3Status>("/api/system/backup/s3"));
    if (!row) throw new Error("secret-shaped payload");
    return row;
  },
  putS3: async (body: S3Write): Promise<S3Status> => {
    const row = asPublicS3(await jsonFetch<S3Status>("/api/system/backup/s3", { method: "PUT", body: JSON.stringify(body) }));
    if (!row) throw new Error("secret-shaped payload");
    return row;
  },
  testS3: () => jsonFetch<{ ok: boolean; configured: boolean }>("/api/system/backup/s3/test", { method: "POST", body: "{}" }),
  clearS3: async (confirm: string): Promise<S3Status> => {
    const row = asPublicS3(await jsonFetch<S3Status>("/api/system/backup/s3/clear", { method: "POST", body: JSON.stringify({ confirm }) }));
    if (!row) throw new Error("secret-shaped payload");
    return row;
  },
};
