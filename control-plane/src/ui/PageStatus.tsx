import type { PageLoadKind } from "../api/page-state";
import { useI18n } from "../i18n";
import { Button } from "./Button";
import { StatusLine } from "./StatusLine";

export function PageStatus({
  kind,
  errorText,
  staleAt,
  onReload,
}: {
  kind: PageLoadKind;
  errorText?: string;
  staleAt?: string;
  onReload?: () => void;
}) {
  const { t } = useI18n();
  if (kind === "loading") {
    return (
      <div data-page-state="loading">
        <StatusLine kind="loading" />
      </div>
    );
  }
  if (kind === "permission") {
    return (
      <div data-page-state="permission">
        <StatusLine kind="error">
          {t("common.permission")}
          {errorText ? ` · ${errorText}` : ""}
        </StatusLine>
      </div>
    );
  }
  if (kind === "error") {
    return (
      <div data-page-state="error">
        <StatusLine kind="error">{errorText || t("common.error")}</StatusLine>
      </div>
    );
  }
  if (kind === "stale") {
    return (
      <div
        role="status"
        data-page-state="stale"
        style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "center", color: "var(--orange)", fontSize: 12.5 }}
      >
        <span>{staleAt ? t("common.staleAt", { at: staleAt }) : t("common.stale")}</span>
        {onReload ? (
          <Button icon="refresh" iconGesture onClick={onReload} style={{ padding: "4px 10px" }}>
            {t("common.reload")}
          </Button>
        ) : null}
      </div>
    );
  }
  return null;
}
