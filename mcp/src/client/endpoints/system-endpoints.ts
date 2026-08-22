/** System endpoints: health, status, models */

import type { HttpClient } from "../http-client.js";
import type { HealthStatus, GatewayStatus, Model } from "../types.js";

export function systemEndpoints(http: HttpClient) {
  return {
    health: () => http.get<HealthStatus>("/health"),
    status: () => http.get<GatewayStatus>("/v1/status"),
    listModels: () => http.get<Model[]>("/v1/models"),
  };
}
