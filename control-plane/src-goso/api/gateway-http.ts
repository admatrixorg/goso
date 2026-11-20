/** Shared gateway HTTP helpers. Default cache avoids reusing the Vite HTML document. */

export const NON_JSON_RESPONSE = "non-JSON response";

/** Default `cache: "no-store"` unless the caller already set `cache`. */
export function gatewayFetchInit(init?: RequestInit): RequestInit {
  return { ...init, cache: init?.cache ?? "no-store" };
}

export function isHtmlGatewayBody(contentType: string | null | undefined, body: string): boolean {
  const ct = (contentType || "").toLowerCase();
  if (ct.includes("text/html")) return true;
  return /^\s*<!doctype/i.test(body);
}

/** Parse a JSON body. HTML 200 (Vite index) is never treated as `{}` / `{agents:[]}`. */
export function parseGatewayJson<T>(contentType: string | null | undefined, body: string): T {
  if (isHtmlGatewayBody(contentType, body)) {
    throw new Error(NON_JSON_RESPONSE);
  }
  return JSON.parse(body) as T;
}

export async function readGatewayJson<T>(res: Response): Promise<T> {
  const text = await res.text();
  if (!res.ok) throw new Error(`${res.status} ${text}`);
  return parseGatewayJson<T>(res.headers.get("content-type"), text);
}

export async function gatewayFetch(
  input: RequestInfo | URL,
  init?: RequestInit,
  fetchImpl: typeof fetch = fetch,
): Promise<Response> {
  return fetchImpl(input, gatewayFetchInit(init));
}
