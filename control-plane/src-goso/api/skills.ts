import { jsonFetch } from "./client";

export type SkillInfo = {
  name: string;
  path?: string;
  score?: number;
  snippet?: string;
};

export const skillsApi = {
  list: () => jsonFetch<{ skills?: SkillInfo[] }>("/api/skills"),
  search: (q: string) => jsonFetch<{ skills?: SkillInfo[] }>(`/api/skills?q=${encodeURIComponent(q)}`),
  create: (body: { name: string; body: string }) =>
    jsonFetch<SkillInfo & { body?: string }>("/api/skills", { method: "POST", body: JSON.stringify(body) }),
  remove: (name: string) => jsonFetch<{ ok?: boolean }>(`/api/skills/${encodeURIComponent(name)}`, { method: "DELETE" }),
};
