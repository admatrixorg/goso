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
  if (hasErr && isPermissionError(err)) {
    return { kind: "permission", showItems: false, showEmpty: false };
  }
  if (input.loaded && input.loading) {
    return { kind: "loading", showItems: count > 0, showEmpty: false };
  }
  if (hasErr && keepStale) {
    return { kind: "stale", showItems: true, showEmpty: false };
  }
  if (hasErr) {
    return { kind: "error", showItems: false, showEmpty: false };
  }
  if (!input.loaded) {
    return { kind: input.loading ? "loading" : "error", showItems: false, showEmpty: false };
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

/** Filtered-empty is distinct from true-empty: inventory has rows, the current filter has none. */
export function isFilteredEmpty(state: PageState, unfilteredCount: number, visibleCount: number): boolean {
  const total = Number.isFinite(unfilteredCount) ? Math.max(0, Math.trunc(unfilteredCount)) : 0;
  const vis = Number.isFinite(visibleCount) ? Math.max(0, Math.trunc(visibleCount)) : 0;
  return state.showItems && total > 0 && vis === 0;
}

/** Blocking inventory never claims a numeric zero; callers render "—". */
export function listMetaCount(kind: PageLoadKind, count: number): number | null {
  if (kind === "error" || kind === "permission" || (kind === "loading" && count <= 0)) return null;
  const n = Number.isFinite(count) ? Math.max(0, Math.trunc(count)) : 0;
  return n;
}

export function clampPageOffset(total: number, offset: number, pageSize: number): number {
  const size = Math.max(1, Math.trunc(Number(pageSize)) || 1);
  const n = Math.max(0, Math.trunc(Number(total)) || 0);
  const off = Math.max(0, Math.trunc(Number(offset)) || 0);
  if (n === 0) return 0;
  if (off >= n) return Math.floor((n - 1) / size) * size;
  return Math.floor(off / size) * size;
}

export function pageSlice<T>(rows: T[], offset: number, pageSize: number): T[] {
  const size = Math.max(1, Math.trunc(Number(pageSize)) || 1);
  const start = clampPageOffset(rows.length, offset, size);
  return rows.slice(start, start + size);
}
