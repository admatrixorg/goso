import { jsonFetch } from "./client";
import {
  asPublicCatalog,
  asPublicJob,
  asPublicPreview,
  type Catalog,
  type Conflict,
  type PortableJob,
  type Preview,
  type Selection,
} from "./impexp-ops";

export type { Catalog, Conflict, PortableJob, Preview, Selection } from "./impexp-ops";

export const impexpApi = {
  catalog: async (): Promise<Catalog> => asPublicCatalog(await jsonFetch<Catalog>("/api/import-export")),
  job: async (id: string): Promise<PortableJob> => {
    const row = asPublicJob(await jsonFetch<PortableJob>(`/api/import-export/${encodeURIComponent(id)}`));
    if (!row) throw new Error("secret-shaped payload");
    return row;
  },
  export: async (sel: Selection): Promise<PortableJob> => {
    const row = asPublicJob(await jsonFetch<PortableJob>("/api/import-export/export", { method: "POST", body: JSON.stringify(sel) }));
    if (!row) throw new Error("secret-shaped payload");
    return row;
  },
  preview: async (archive: unknown): Promise<Preview> =>
    asPublicPreview(await jsonFetch<Preview>("/api/import-export/preview", { method: "POST", body: JSON.stringify({ archive }) })),
  import: async (archive: unknown, conflict: Conflict, dryRun: boolean): Promise<PortableJob> => {
    const row = asPublicJob(
      await jsonFetch<PortableJob>("/api/import-export/import", {
        method: "POST",
        body: JSON.stringify({ archive, conflict, dry_run: dryRun }),
      }),
    );
    if (!row) throw new Error("secret-shaped payload");
    return row;
  },
  rollback: async (id: string): Promise<PortableJob> => {
    const row = asPublicJob(await jsonFetch<PortableJob>(`/api/import-export/${encodeURIComponent(id)}/rollback`, { method: "POST", body: "{}" }));
    if (!row) throw new Error("secret-shaped payload");
    return row;
  },
};
