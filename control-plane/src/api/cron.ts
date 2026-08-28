import { jsonFetch } from "./client";

export type CronJob = {
  id: string;
  spec: string;
  session_id: string;
  message: string;
  enabled?: boolean;
  last_run?: string;
};

export const cronApi = {
  list: () => jsonFetch<{ jobs?: CronJob[] }>("/api/cron"),
  create: (body: { spec: string; session_id: string; message: string }) =>
    jsonFetch<CronJob>("/api/cron", { method: "POST", body: JSON.stringify(body) }),
  remove: (id: string) => jsonFetch<{ ok?: boolean }>(`/api/cron/${encodeURIComponent(id)}`, { method: "DELETE" }),
};
