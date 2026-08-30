export type PageLoadKind = "loading" | "empty" | "error" | "permission" | "stale" | "ready";

function errorStatus(e: unknown): number {
  const s = e instanceof Error ? e.message : String(e);
  const m = s.match(/^(\d{3})\b/);
  return m ? Number(m[1]) : 0;
}

function isUnauthorizedStatus(status: number): boolean {
  return status === 401 || status === 403;
}

export type PageState = {
  kind: PageLoadKind;
  showItems: boolean;
  showEmpty: boolean;
};

export function isPermissionError(err: unknown): boolean {
  return isUnauthorizedStatus(errorStatus(err));
}

/** Create/mutate entry points stay closed while required inventory is blocking. */
export function inventoryBlocksMutation(kind: PageLoadKind): boolean {
  return kind === "error" || kind === "permission";
}

/**
 * Classify a list/page load. Empty is only true after a successful load with
 * zero items and no error. Permission/error never claim emptiness. Stale keeps
 * last-known items only when keepStale is set.
 */
export function classifyPageState(input: {
  loading: boolean;
  loaded: boolean;
  error: unknown | null | undefined;
  itemCount: number;
  keepStale?: boolean;
}): PageState {
  const count = Number.isFinite(input.itemCount) ? Math.max(0, Math.trunc(input.itemCount)) : 0;
  const err = input.error ?? null;
  const hasErr = Boolean(err);
  const keepStale = input.keepStale === true && input.loaded && count > 0;

  if (!input.loaded && input.loading) {
    return { kind: "loading", showItems: false, showEmpty: false };
  }
  if (hasErr && keepStale) {
    return { kind: "stale", showItems: true, showEmpty: false };
  }
  if (hasErr && isPermissionError(err)) {
    return { kind: "permission", showItems: false, showEmpty: false };
  }
  if (hasErr) {
    return { kind: "error", showItems: false, showEmpty: false };
  }
  if (!input.loaded) {
    return { kind: input.loading ? "loading" : "error", showItems: false, showEmpty: false };
  }
  if (input.loading) {
    return { kind: "loading", showItems: count > 0, showEmpty: false };
  }
  if (count === 0) {
    return { kind: "empty", showItems: false, showEmpty: true };
  }
  return { kind: "ready", showItems: true, showEmpty: false };
}

export function formatStaleAt(iso: string | null | undefined, locale: string): string {
  const raw = (iso || "").trim();
  if (!raw) return "";
  const d = new Date(raw);
  if (Number.isNaN(d.getTime())) return raw;
  try {
    return d.toLocaleString(locale === "en" ? "en-US" : "vi-VN");
  } catch {
    return raw;
  }
}
