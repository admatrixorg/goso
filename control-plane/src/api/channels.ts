import { jsonFetch } from "./client";

export type ChannelRow = {
  name: string;
  configured: boolean;
  missing: boolean;
  env: string;
  env_names: string[];
};

export const channelsApi = {
  list: () => jsonFetch<{ channels: ChannelRow[]; lite?: boolean }>("/api/channels"),
};
