import type { ButtonHTMLAttributes, CSSProperties, ReactNode } from "react";
import { Icon, type IconName } from "./Icon";

const shells: Record<string, CSSProperties> = {
  primary: { background: "var(--btn-dark-bg)", color: "var(--btn-dark-fg)", border: "1px solid transparent", fontWeight: 600 },
  secondary: { background: "var(--card)", color: "var(--text)", border: "1px solid var(--border)", fontWeight: 600 },
  quiet: { background: "transparent", color: "var(--text-2)", border: "1px solid var(--border)", fontWeight: 500 },
  ghost: { background: "transparent", color: "var(--text-2)", border: "1px solid transparent", fontWeight: 500 },
  accent: { background: "var(--accent)", color: "#fff", border: "1px solid transparent", fontWeight: 600 },
};

export function Button({
  variant = "secondary",
  icon,
  iconGesture,
  children,
  disabled,
  style,
  ...rest
}: {
  variant?: "primary" | "secondary" | "quiet" | "ghost" | "accent";
  icon?: IconName;
  iconGesture?: boolean;
  children?: ReactNode;
} & ButtonHTMLAttributes<HTMLButtonElement>) {
  return (
    <button
      type="button"
      disabled={disabled}
      data-ig={iconGesture ? "refresh" : undefined}
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: "var(--space-5)",
        padding: "var(--pad-btn)",
        borderRadius: "var(--radius-xl)",
        fontFamily: "var(--font-sans)",
        fontSize: "var(--fs-body)",
        lineHeight: 1.2,
        whiteSpace: "nowrap",
        cursor: disabled ? "default" : "pointer",
        opacity: disabled ? 0.45 : 1,
        transition: "background var(--dur-control) var(--ease-standard), color var(--dur-control) var(--ease-standard), border-color var(--dur-control) var(--ease-standard)",
        ...shells[variant],
        ...style,
      }}
      {...rest}
    >
      {icon ? <Icon name={icon} size={14} /> : null}
      {children}
    </button>
  );
}
