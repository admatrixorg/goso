import { jsonFetch } from "./client";

export type BackupFile = {
  file: string;
  bytes: number;
  integrity: string;
  mtime?: string;
};

export type BackupList = { files: BackupFile[] };

export const backupApi = {
  list: () => jsonFetch<BackupList>("/api/system/backup"),
  create: () => jsonFetch<BackupFile>("/api/system/backup", { method: "POST", body: "{}" }),
  restore: (file: string) =>
    jsonFetch<{ file: string; integrity: string; applied: boolean }>("/api/system/restore", {
      method: "POST",
      body: JSON.stringify({ file }),
    }),
};
