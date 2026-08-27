import type { ReactNode } from "react";
import { useI18n } from "../i18n";
import { EmptyState } from "./EmptyState";

/** Shared loading / error row. Empty copy stays on EmptyState per page. */
export function StatusLine({ kind, children }: { kind: "loading" | "error"; children?: ReactNode }) {
  const { t } = useI18n();
  if (kind === "loading") {
    return <EmptyState role="status">{children ?? t("common.loading")}</EmptyState>;
  }
  return (
    <p role="alert" style={{ color: "var(--red)", fontSize: 12.5, margin: 0 }}>
      {children ?? t("common.error")}
    </p>
  );
}

const MAX = 400;

/** Surface gateway status text (502 / LLM 401). Truncate; never echo secrets. */
export function formatPublicError(e: unknown): string {
  let s = String(e);
  s = s
    .replace(/Bearer\s+\S+/gi, "Bearer [redacted]")
    .replace(/\bsk-[A-Za-z0-9_*-]+\b/g, "sk-[redacted]")
    .replace(/\bgsk_[A-Za-z0-9]+\b/g, "gsk_[redacted]")
    .replace(/\bxai-[A-Za-z0-9]+\b/g, "xai-[redacted]")
    .replace(/\bAIza[A-Za-z0-9_-]+\b/g, "AIza[redacted]")
    .replace(/\bwh_[A-Za-z0-9]+\b/g, "wh_[redacted]")
    .replace(/"(authorization|api[_-]?key|secret|hmac(?:_key)?|token)"\s*:\s*"[^"]*"/gi, '"$1":"[redacted]"')
    .replace(/(authorization|api[_-]?key|secret|hmac(?:_key)?|token)\s*[:=]\s*\S+/gi, "$1=[redacted]");
  if (s.length > MAX) s = `${s.slice(0, MAX)}…`;
  return s;
}
