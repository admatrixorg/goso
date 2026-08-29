import { jsonFetch } from "./client";

/** GET /api/workstations — SSH/Docker targets. Identity is a path/ref, never a private key. */
export type Workstation = {
  id: string;
  display: string;
  backend: string;
  host: string;
  port: number;
  user?: string;
  identity_ref?: string;
  identity_set: boolean;
  agent_id?: string;
  health: string;
  last_tested?: string;
  created_at?: string;
  updated_at?: string;
};

export type WorkstationList = {
  workstations: Workstation[];
};

export type WorkstationWrite = {
  display: string;
  backend: string;
  host: string;
  port?: number;
  user?: string;
  identity_ref?: string;
  agent_id?: string;
};

export type WorkstationTest = {
  ok: boolean;
  health: string;
  summary: string;
  backend: string;
  host: string;
  port: number;
  identity_set: boolean;
};

export const workstationsApi = {
  list: () => jsonFetch<WorkstationList>("/api/workstations"),
  get: (id: string) => jsonFetch<Workstation>(`/api/workstations/${encodeURIComponent(id)}`),
  create: (body: WorkstationWrite) =>
    jsonFetch<Workstation>("/api/workstations", { method: "POST", body: JSON.stringify(body) }),
  patch: (id: string, body: Partial<WorkstationWrite>) =>
    jsonFetch<Workstation>(`/api/workstations/${encodeURIComponent(id)}`, {
      method: "PATCH",
      body: JSON.stringify(body),
    }),
  test: (id: string) =>
    jsonFetch<WorkstationTest>(`/api/workstations/${encodeURIComponent(id)}/test`, { method: "POST" }),
  disconnect: (id: string, confirm: string) =>
    jsonFetch<Workstation>(`/api/workstations/${encodeURIComponent(id)}/disconnect`, {
      method: "POST",
      body: JSON.stringify({ confirm }),
    }),
  remove: (id: string, confirm: string) =>
    jsonFetch<Workstation>(`/api/workstations/${encodeURIComponent(id)}/delete`, {
      method: "POST",
      body: JSON.stringify({ confirm }),
    }),
};
