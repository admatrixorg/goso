import { jsonFetch } from "./client";

export type CronJob = {
  id: string;
  spec: string;
  session_id: string;
  message: string;
  enabled?: boolean;
  last_run?: string;
  last_error?: string;
};

export const cronApi = {
  list: () => jsonFetch<{ jobs?: CronJob[] }>("/api/cron"),
  create: (body: { spec: string; session_id: string; message: string; enabled?: boolean }) =>
    jsonFetch<CronJob>("/api/cron", { method: "POST", body: JSON.stringify(body) }),
  setEnabled: (id: string, enabled: boolean) =>
    jsonFetch<CronJob>(`/api/cron/${encodeURIComponent(id)}`, {
      method: "PATCH",
      body: JSON.stringify({ enabled }),
    }),
  remove: (id: string) => jsonFetch<{ ok?: boolean }>(`/api/cron/${encodeURIComponent(id)}`, { method: "DELETE" }),
};
