import { jsonFetch } from "./client";
import {
  asPublicStatus,
  type TTSStatus,
  type TTSTest,
  type TTSWrite,
} from "./tts-ops";

export type { TTSStatus, TTSTest, TTSWrite } from "./tts-ops";
export {
  TTS_APPLY,
  TTS_PROVIDERS,
  asPublicStatus,
  emptyStatus,
  formatTTSTest,
  parseTTSTestError,
  publicHasSecrets,
  requiresKey,
  statusKind,
  ttsConfirmMatch,
  ttsWriteBody,
} from "./tts-ops";

export const ttsApi = {
  get: async (): Promise<TTSStatus> => {
    const row = asPublicStatus(await jsonFetch<TTSStatus>("/api/tts"));
    if (!row) throw new Error("secret-shaped payload");
    return row;
  },
  put: async (body: TTSWrite): Promise<TTSStatus> => {
    const row = asPublicStatus(await jsonFetch<TTSStatus>("/api/tts", { method: "PUT", body: JSON.stringify(body) }));
    if (!row) throw new Error("secret-shaped payload");
    return row;
  },
  test: () => jsonFetch<TTSTest>("/api/tts/test", { method: "POST", body: "{}" }),
  clear: async (confirm: string): Promise<TTSStatus> => {
    const row = asPublicStatus(
      await jsonFetch<TTSStatus>("/api/tts/clear", { method: "POST", body: JSON.stringify({ confirm }) }),
    );
    if (!row) throw new Error("secret-shaped payload");
    return row;
  },
};
