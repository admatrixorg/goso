/** Exact named target for typed destructive confirms. Blank expected never matches. */
export function namedConfirmTarget(expected: string, typed: string): boolean {
  return (typed || "").trim() === (expected || "").trim() && Boolean((expected || "").trim());
}

export type ConfirmFn = (message: string) => boolean;

/** Named confirm via window.confirm (or injected fn). Cancel/false is a no-op. */
export function confirmNamed(message: string, confirmFn: ConfirmFn): boolean {
  return confirmFn(message) === true;
}

export type TypedConfirmResult = "ok" | "cancel" | "mismatch";

/** Typed-name confirm. null from prompt is cancel. */
export function typedConfirm(expected: string, typed: string | null | undefined): TypedConfirmResult {
  if (typed == null) return "cancel";
  if (!namedConfirmTarget(expected, typed)) return "mismatch";
  return "ok";
}
