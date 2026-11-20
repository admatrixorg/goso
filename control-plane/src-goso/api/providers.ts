import { jsonFetch } from "./client";
export type {
  ProviderEnabledFilter,
  ProviderInfo,
  ProviderSourceFilter,
  ProviderTestResult,
  ProviderTestView,
  ProviderWrite,
} from "./provider-ops";
export {
  PROVIDER_TYPES,
  canClearProviderKey,
  filterProviders,
  formatProviderTest,
  isEnvOwned,
  isProviderEnabled,
  providerWriteBody,
  uniqueProviderTypes,
} from "./provider-ops";
import type { ProviderInfo, ProviderTestResult, ProviderWrite } from "./provider-ops";

export const providersApi = {
  list: () => jsonFetch<{ providers: ProviderInfo[] }>("/api/providers"),
  create: (body: ProviderWrite) =>
    jsonFetch<ProviderInfo>("/api/providers", { method: "POST", body: JSON.stringify(body) }),
  patch: (name: string, body: ProviderWrite) =>
    jsonFetch<ProviderInfo>(`/api/providers/${encodeURIComponent(name)}`, {
      method: "PATCH",
      body: JSON.stringify(body),
    }),
  clearKey: (name: string) =>
    jsonFetch<{ ok: boolean; name: string; key_set: boolean; source: string }>(
      `/api/providers/${encodeURIComponent(name)}/key`,
      { method: "DELETE" },
    ),
  test: (name: string, kind?: "models" | "chat") =>
    jsonFetch<ProviderTestResult>(`/api/providers/${encodeURIComponent(name)}/test`, {
      method: "POST",
      body: JSON.stringify(kind ? { kind } : {}),
    }),
};
