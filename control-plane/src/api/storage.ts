import { jsonFetch, TENANT_STORAGE_KEY } from "./client";
import { gatewayFetchInit, readGatewayJson } from "./gateway-http";

export type StorageCrumb = { name: string; path: string };

export type StorageEntry = {
  name: string;
  path: string;
  dir: boolean;
  size: number;
  type: string;
  mtime?: string;
};

export type StorageListing = {
  configured: boolean;
  path: string;
  parent?: string;
  breadcrumbs: StorageCrumb[];
  entries: StorageEntry[];
  used_bytes: number;
  max_bytes: number;
  hidden_skipped: number;
  truncated?: boolean;
};

export type StoragePreview = {
  path: string;
  type: string;
  size: number;
  kind: "text" | "binary" | "denied" | string;
  text?: string;
  truncated?: boolean;
  bytes: number;
};

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

async function blobFetch(path: string): Promise<Blob> {
  const res = await fetch(`${gatewayBase()}${path}`, gatewayFetchInit({ headers: authHeaders() }));
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`${res.status} ${text}`);
  }
  return res.blob();
}

export const storageApi = {
  list: (path = "", showHidden = false) => {
    const q = new URLSearchParams();
    if (path) q.set("path", path);
    if (showHidden) q.set("show_hidden", "1");
    const suffix = q.toString() ? `?${q.toString()}` : "";
    return jsonFetch<StorageListing>(`/api/storage${suffix}`);
  },
  preview: (path: string) =>
    jsonFetch<StoragePreview>(`/api/storage/preview?path=${encodeURIComponent(path)}`),
  download: (path: string) => blobFetch(`/api/storage/download?path=${encodeURIComponent(path)}`),
  upload: async (file: File, path = "") => {
    const body = new FormData();
    body.append("file", file, file.name);
    if (path) body.append("path", path);
    const res = await fetch(
      `${gatewayBase()}/api/storage/upload`,
      gatewayFetchInit({
        method: "POST",
        headers: authHeaders(),
        body,
      }),
    );
    return readGatewayJson<StorageEntry>(res);
  },
  remove: (path: string, confirm: string) =>
    jsonFetch<StorageEntry>("/api/storage/delete", {
      method: "POST",
      body: JSON.stringify({ path, confirm }),
    }),
};
