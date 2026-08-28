import { jsonFetch } from "./client";

export type ChannelRow = { name: string; configured: boolean; env: string };

export const channelsApi = {
  list: () => jsonFetch<{ channels: ChannelRow[]; lite?: boolean }>("/api/channels"),
};
