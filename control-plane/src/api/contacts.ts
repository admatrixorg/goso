import { jsonFetch } from "./client";

/** GET /api/contacts — identities and metadata only. Never tokens. */
export type ContactIdent = {
  channel: string;
  dest: string;
  kind: string;
  permission: string;
};

export type Contact = {
  id: string;
  display: string;
  kind: string;
  channel: string;
  dest: string;
  identifiers: ContactIdent[];
  count: number;
  first_seen?: string;
  last_seen?: string;
  permission?: string;
  agent_id?: string;
  agent?: string;
  can_undo?: boolean;
  merged_from?: string[];
};

export const contactsApi = {
  list: (q?: { q?: string; channel?: string; kind?: string }) => {
    const p = new URLSearchParams();
    if (q?.q) p.set("q", q.q);
    if (q?.channel) p.set("channel", q.channel);
    if (q?.kind) p.set("kind", q.kind);
    const qs = p.toString();
    return jsonFetch<{ contacts: Contact[]; total: number }>(`/api/contacts${qs ? `?${qs}` : ""}`);
  },
  get: (id: string) => jsonFetch<Contact>(`/api/contacts/${encodeURIComponent(id)}`),
  merge: (id: string, sourceId: string, confirm: string) =>
    jsonFetch<Contact>(`/api/contacts/${encodeURIComponent(id)}/merge`, {
      method: "POST",
      body: JSON.stringify({ source_id: sourceId, confirm }),
    }),
  undo: (id: string, confirm: string) =>
    jsonFetch<Contact>(`/api/contacts/${encodeURIComponent(id)}/undo`, {
      method: "POST",
      body: JSON.stringify({ confirm }),
    }),
};
