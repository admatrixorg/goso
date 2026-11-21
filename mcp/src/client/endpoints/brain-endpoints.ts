import type { HttpClient } from "../http-client.js";

export function brainEndpoints(http: HttpClient) {
  return {
    listPages: (tenant: string) => http.get<{ tenant: string; pages: unknown[] }>(`/v1/brain/pages?tenant=${encodeURIComponent(tenant)}`),
    getPage: (tenant: string, id: string) => http.get<unknown>(`/v1/brain/pages/${encodeURIComponent(id)}?tenant=${encodeURIComponent(tenant)}`),
    search: (tenant: string, q: string) => http.get<{ hits: unknown[] }>(`/v1/brain/search?tenant=${encodeURIComponent(tenant)}&q=${encodeURIComponent(q)}`),
    listSources: (tenant: string) => http.get<{ sources: unknown[] }>(`/v1/brain/sources?tenant=${encodeURIComponent(tenant)}`),
    graph: (tenant: string) => http.get<{ edges: unknown[] }>(`/v1/brain/graph?tenant=${encodeURIComponent(tenant)}`),
    compile: (tenant: string) => http.post<unknown>("/v1/brain/compile", { tenant }),
  };
}
