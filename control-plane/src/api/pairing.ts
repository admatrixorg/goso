import { jsonFetch } from "./client";

export type PairingIssued = {
  code: string;
  expires_at: string;
  ttl_seconds: number;
  role: string;
};

export const pairingApi = {
  create: () => jsonFetch<PairingIssued>("/api/pairing", { method: "POST", body: "{}" }),
};
