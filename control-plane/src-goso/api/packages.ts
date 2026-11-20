import { jsonFetch } from "./client";
import { asPublicCreds, asPublicSnapshot, type AllowEntry, type CLICred, type Pkg, type PkgJob, type Snapshot } from "./packages-ops";

export type { AllowEntry, CLICred, Pkg, PkgJob, Snapshot } from "./packages-ops";

export type PkgAction = { package: Pkg; job: PkgJob };

export const packagesApi = {
  snapshot: async (): Promise<Snapshot> => {
    const j = await jsonFetch<Snapshot>("/api/packages");
    return asPublicSnapshot(j);
  },
  allow: (body: { ecosystem: string; name: string; pin: string }) =>
    jsonFetch<AllowEntry>("/api/packages/allow", { method: "POST", body: JSON.stringify(body) }),
  unpin: (id: string, confirm: string) =>
    jsonFetch<AllowEntry>("/api/packages/unpin", { method: "POST", body: JSON.stringify({ id, confirm }) }),
  install: (body: { ecosystem: string; name: string; version: string; confirm: string }) =>
    jsonFetch<PkgAction>("/api/packages/install", { method: "POST", body: JSON.stringify(body) }),
  uninstall: (id: string, confirm: string) =>
    jsonFetch<PkgAction>(`/api/packages/${encodeURIComponent(id)}/uninstall`, {
      method: "POST",
      body: JSON.stringify({ confirm }),
    }),
  recover: (id: string, confirm: string) =>
    jsonFetch<PkgAction>(`/api/packages/${encodeURIComponent(id)}/recover`, {
      method: "POST",
      body: JSON.stringify({ confirm }),
    }),
  setCLI: async (kind: string, token: string): Promise<CLICred> => {
    const row = await jsonFetch<CLICred>("/api/packages/cli", {
      method: "POST",
      body: JSON.stringify({ kind, token }),
    });
    return asPublicCreds([row])[0] ?? { kind, set: false };
  },
  clearCLI: async (kind: string, confirm: string): Promise<CLICred> => {
    const row = await jsonFetch<CLICred>("/api/packages/uncli", {
      method: "POST",
      body: JSON.stringify({ kind, confirm }),
    });
    return asPublicCreds([row])[0] ?? { kind, set: false };
  },
};
