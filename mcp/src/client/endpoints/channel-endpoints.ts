/** Channel endpoints: list, toggle */

import type { HttpClient } from "../http-client.js";
import type { Channel } from "../types.js";

export function channelEndpoints(http: HttpClient) {
  return {
    listChannels: () => http.get<Channel[]>("/v1/channels"),
    toggleChannel: (channel: string, enabled: boolean) =>
      http.patch<Channel>(`/v1/channels/${encodeURIComponent(channel)}`, { enabled }),
  };
}
