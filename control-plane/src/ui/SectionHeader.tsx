import type { HTMLAttributes, ReactNode } from "react";
import { Icon, type IconName } from "./Icon";

export function SectionHeader({
  icon,
  title,
  description,
  actions,
  style,
  ...rest
}: {
  icon?: IconName;
  title: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
} & HTMLAttributes<HTMLDivElement>) {
  return (
    <div style={{ display: "flex", gap: "var(--space-9)", alignItems: "flex-start", ...style }} {...rest}>
      {icon ? (
        <div
          style={{
            width: "var(--size-icon-badge)",
            height: "var(--size-icon-badge)",
            flex: "none",
            borderRadius: "var(--radius-2xl)",
            background: "var(--accent)",
            color: "#fff",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
          }}
        >
          <Icon name={icon} size={22} />
        </div>
      ) : null}
      <div style={{ flex: 1 }}>
        <div style={{ fontSize: "var(--fs-page-title)", fontWeight: 700, letterSpacing: "var(--ls-title)" }}>{title}</div>
        {description ? (
          <div style={{ fontSize: "var(--fs-meta)", color: "var(--text-3)", maxWidth: 640, textWrap: "pretty" as const }}>
            {description}
          </div>
        ) : null}
      </div>
      {actions ? <div style={{ display: "flex", gap: "var(--space-6)", flexWrap: "wrap" }}>{actions}</div> : null}
    </div>
  );
}
