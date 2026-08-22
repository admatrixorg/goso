/** Cron job endpoints: CRUD + toggle + run */

import type { HttpClient } from "../http-client.js";
import type { CronJob, CreateCronRequest, UpdateCronRequest } from "../types.js";

export function cronEndpoints(http: HttpClient) {
  return {
    listCronJobs: () => http.get<CronJob[]>("/v1/cron"),
    createCronJob: (data: CreateCronRequest) => http.post<CronJob>("/v1/cron", data),
    updateCronJob: (id: string, data: UpdateCronRequest) =>
      http.put<CronJob>(`/v1/cron/${id}`, data),
    deleteCronJob: (id: string) => http.del(`/v1/cron/${id}`),
    toggleCronJob: (id: string, enabled: boolean) =>
      http.patch<CronJob>(`/v1/cron/${id}`, { enabled }),
    runCronJob: (id: string) => http.post(`/v1/cron/${id}/run`),
  };
}
