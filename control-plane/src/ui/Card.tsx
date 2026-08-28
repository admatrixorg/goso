import type { HTMLAttributes, ReactNode } from "react";
import { Icon, type IconName } from "./Icon";

export function Card({ children, style, ...rest }: { children?: ReactNode } & HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      style={{
        background: "var(--card)",
        border: "var(--border-card)",
        borderRadius: "var(--radius-card)",
        minWidth: 0,
        ...style,
      }}
      {...rest}
    >
      {children}
    </div>
  );
}

export function TableScroll({ children }: { children?: ReactNode }) {
  return (
    <div className="z-scroll-x">
      <div className="z-row-table">{children}</div>
    </div>
  );
}

export function CardHeader({
  icon,
  iconColor = "var(--accent)",
  title,
  meta,
  style,
  ...rest
}: {
  icon?: IconName;
  iconColor?: string;
  title: ReactNode;
  meta?: ReactNode;
} & HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      style={{
        display: "flex",
        alignItems: "center",
        gap: "var(--space-6)",
        padding: "var(--pad-card-header)",
        borderBottom: "var(--border-divider)",
        fontFamily: "var(--font-sans)",
        fontSize: "var(--fs-card-title)",
        fontWeight: 600,
        ...style,
      }}
      {...rest}
    >
      {icon ? (
        <span style={{ color: iconColor, display: "flex" }}>
          <Icon name={icon} size={15} />
        </span>
      ) : null}
      <span>{title}</span>
      {meta ? (
        <span style={{ marginLeft: "auto", fontSize: "var(--fs-badge)", color: "var(--text-3)", fontWeight: 400 }}>
          {meta}
        </span>
      ) : null}
    </div>
  );
}
