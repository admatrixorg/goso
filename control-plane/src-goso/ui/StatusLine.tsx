import type { ReactNode } from "react";
import { useI18n } from "../i18n";

/** Shared loading / error row. Empty collections use EmptyState, not this. */
export function StatusLine({ kind, children }: { kind: "loading" | "error"; children?: ReactNode }) {
  const { t } = useI18n();
  if (kind === "loading") {
    return (
      <p role="status" style={{ color: "var(--muted-foreground)", fontSize: 14, margin: 0, padding: "12px 4px" }}>
        {children ?? t("common.loading")}
      </p>
    );
  }
  return (
    <p role="alert" style={{ color: "var(--destructive)", fontSize: 14, margin: 0 }}>
      {children ?? t("common.error")}
    </p>
  );
}

export { crmPermissionBanner, formatCrmPublicError, formatPublicError, redactPublicText } from "../api/public-error";
