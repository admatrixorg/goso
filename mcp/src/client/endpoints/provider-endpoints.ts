/** LLM Provider endpoints: CRUD */

import type { HttpClient } from "../http-client.js";
import type { Provider, CreateProviderRequest, UpdateProviderRequest } from "../types.js";

export function providerEndpoints(http: HttpClient) {
  return {
    listProviders: () => http.get<Provider[]>("/v1/providers"),
    getProvider: (id: string) => http.get<Provider>(`/v1/providers/${id}`),
    createProvider: (data: CreateProviderRequest) =>
      http.post<Provider>("/v1/providers", data),
    updateProvider: (id: string, data: UpdateProviderRequest) =>
      http.put<Provider>(`/v1/providers/${id}`, data),
    deleteProvider: (id: string) => http.del(`/v1/providers/${id}`),
  };
}
