import { jsonFetch } from "./client";

export type ProviderInfo = {
  name: string;
  type: string;
  base_url: string;
  model: string;
  key_set: boolean;
  source: "env" | "sqlite" | string;
};

export type ProviderTestResult = {
  ok: boolean;
  latency_ms: number;
  models?: string[];
  reply?: string;
  error?: string;
};

export type ProviderWrite = {
  name?: string;
  type?: string;
  base_url?: string;
  model?: string;
  api_key?: string;
};

export const providersApi = {
  list: () => jsonFetch<{ providers: ProviderInfo[] }>("/api/providers"),
  create: (body: ProviderWrite) =>
    jsonFetch<ProviderInfo>("/api/providers", { method: "POST", body: JSON.stringify(body) }),
  patch: (name: string, body: ProviderWrite) =>
    jsonFetch<ProviderInfo>(`/api/providers/${encodeURIComponent(name)}`, {
      method: "PATCH",
      body: JSON.stringify(body),
    }),
  test: (name: string, kind?: "models" | "chat") =>
    jsonFetch<ProviderTestResult>(`/api/providers/${encodeURIComponent(name)}/test`, {
      method: "POST",
      body: JSON.stringify(kind ? { kind } : {}),
    }),
};
