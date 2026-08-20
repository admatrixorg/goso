/** Config endpoints: get, apply, patch */

import type { HttpClient } from "../http-client.js";
import type { GatewayConfig } from "../types.js";

export function configEndpoints(http: HttpClient) {
  return {
    getConfig: (section?: string) => {
      const qs = section ? `?section=${encodeURIComponent(section)}` : "";
      return http.get<GatewayConfig>(`/v1/config${qs}`);
    },
    applyConfig: (config: GatewayConfig) =>
      http.post<GatewayConfig>("/v1/config", config),
    patchConfig: (patches: GatewayConfig) =>
      http.patch<GatewayConfig>("/v1/config", patches),
    getConfigSchema: () => http.get<Record<string, unknown>>("/v1/config/schema"),
  };
}
