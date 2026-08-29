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

export { formatPublicError, redactPublicText } from "../api/public-error";
