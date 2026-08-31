/** Browser-session admin bearer for Control Plane. Write-only; never hydrate. */

export const BROWSER_TOKEN_STORAGE_KEY = "goso_token";
export const BROWSER_TOKEN_PROBE_KEY = "goso_browser_token_probe";

export type BrowserTokenKind = "env-owned" | "set" | "unset";
export type BrowserTokenProbe = "accepted" | "unauthorized" | "unreachable";

export type BrowserTokenStore = {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
  removeItem(key: string): void;
};

export type BrowserTokenEnv = {
  viteAdminToken?: string;
};

export type SaveBrowserTokenResult =
  | { ok: true; input: ""; reload: true }
  | { ok: false; reason: "empty" | "env-owned"; input: string; reload: false };

export type ClearBrowserTokenResult =
  | { ok: true; input: ""; reload: true }
  | { ok: false; reason: "env-owned" | "unset"; input: string; reload: false };

const PROBE_TIMEOUT_MS = 5000;

/** Password field starts empty. Never read localStorage, env, or GET. */
export function emptyBrowserTokenInput(): string {
  return "";
}

export function hydrateBrowserTokenInput(_store?: BrowserTokenStore, _env?: BrowserTokenEnv): string {
  return emptyBrowserTokenInput();
}

function envToken(env: BrowserTokenEnv): string {
  return env.viteAdminToken || "";
}

function storedToken(store: BrowserTokenStore): string {
  try {
    return (store.getItem(BROWSER_TOKEN_STORAGE_KEY) || "").trim();
  } catch {
    return "";
  }
}

export function browserTokenKind(env: BrowserTokenEnv, store: BrowserTokenStore): BrowserTokenKind {
  if (envToken(env)) return "env-owned";
  return storedToken(store) ? "set" : "unset";
}

export function browserTokenWritable(kind: BrowserTokenKind): boolean {
  return kind !== "env-owned";
}

export function browserTokenClearable(kind: BrowserTokenKind): boolean {
  return kind === "set";
}

/** Always shown on Config → Gateway → Auth, including 401/blocked inventory. */
export function browserTokenControlVisible(_inventoryKind?: string): boolean {
  return true;
}

export function saveBrowserToken(input: string, env: BrowserTokenEnv, store: BrowserTokenStore): SaveBrowserTokenResult {
  if (browserTokenKind(env, store) === "env-owned") {
    return { ok: false, reason: "env-owned", input, reload: false };
  }
  const value = input.trim();
  if (!value) {
    return { ok: false, reason: "empty", input, reload: false };
  }
  store.setItem(BROWSER_TOKEN_STORAGE_KEY, value);
  return { ok: true, input: "", reload: true };
}

export function clearBrowserToken(env: BrowserTokenEnv, store: BrowserTokenStore, input = ""): ClearBrowserTokenResult {
  const kind = browserTokenKind(env, store);
  if (kind === "env-owned") {
    return { ok: false, reason: "env-owned", input, reload: false };
  }
  if (kind !== "set") {
    return { ok: false, reason: "unset", input, reload: false };
  }
  store.removeItem(BROWSER_TOKEN_STORAGE_KEY);
  return { ok: true, input: "", reload: true };
}

export function classifyTokenProbeStatus(status: number): BrowserTokenProbe {
  if (status === 200) return "accepted";
  if (status === 401 || status === 403) return "unauthorized";
  return "unreachable";
}

export type ProbeFetch = (
  input: string,
  init?: { method?: string; cache?: RequestCache; headers?: Record<string, string>; signal?: AbortSignal },
) => Promise<{ status: number; text: () => Promise<string> }>;

/** Status-only probe of GET /api/agents. Body is discarded and never returned. */
export async function probeBrowserToken(
  fetchImpl: ProbeFetch,
  baseUrl: string,
  token: string,
): Promise<BrowserTokenProbe> {
  const ctrl = new AbortController();
  const timer = setTimeout(() => ctrl.abort(), PROBE_TIMEOUT_MS);
  try {
    const headers: Record<string, string> = {};
    if (token) headers.Authorization = `Bearer ${token}`;
    const res = await fetchImpl(`${baseUrl.replace(/\/$/, "")}/api/agents`, {
      method: "GET",
      cache: "no-store",
      headers,
      signal: ctrl.signal,
    });
    try {
      await res.text();
    } catch {
      /* discard */
    }
    return classifyTokenProbeStatus(res.status);
  } catch {
    return "unreachable";
  } finally {
    clearTimeout(timer);
  }
}

export function writeBrowserTokenProbe(store: BrowserTokenStore, result: BrowserTokenProbe): void {
  try {
    store.setItem(BROWSER_TOKEN_PROBE_KEY, result);
  } catch {
    /* private mode */
  }
}

export function consumeBrowserTokenProbe(store: BrowserTokenStore): BrowserTokenProbe | "" {
  try {
    const raw = store.getItem(BROWSER_TOKEN_PROBE_KEY) || "";
    store.removeItem(BROWSER_TOKEN_PROBE_KEY);
    if (raw === "accepted" || raw === "unauthorized" || raw === "unreachable") return raw;
    return "";
  } catch {
    return "";
  }
}
