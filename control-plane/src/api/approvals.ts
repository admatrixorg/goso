import { jsonFetch } from "./client";
import { asPublic, asPublicList, type Approval, type ApprovalList } from "./approvals-ops";

export type { Approval, ApprovalList } from "./approvals-ops";

export const approvalsApi = {
  list: async (): Promise<ApprovalList> => {
    const j = await jsonFetch<ApprovalList>("/api/approvals");
    return asPublicList(j);
  },
  get: async (id: string): Promise<Approval> => {
    const row = await jsonFetch<Approval>(`/api/approvals/${encodeURIComponent(id)}`);
    const pub = asPublic([row])[0];
    if (!pub) throw new Error("secret-shaped payload");
    return pub;
  },
  decide: (id: string, decision: "approve" | "deny", reason?: string) =>
    jsonFetch<Approval>(`/api/approvals/${encodeURIComponent(id)}/decision`, {
      method: "POST",
      body: JSON.stringify(decision === "deny" ? { decision, reason } : { decision: "approve" }),
    }),
};
