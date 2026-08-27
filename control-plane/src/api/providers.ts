import { jsonFetch } from "./client";

export const providersApi = {
  list: () => jsonFetch<{ providers: string[] }>("/api/providers"),
};
