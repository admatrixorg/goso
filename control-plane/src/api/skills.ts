import { jsonFetch } from "./client";

export type SkillInfo = {
  name: string;
  path: string;
};

export const skillsApi = {
  list: () => jsonFetch<{ skills?: SkillInfo[] }>("/api/skills"),
};
