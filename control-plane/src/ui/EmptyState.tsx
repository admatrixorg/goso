import type { HTMLAttributes, ReactNode } from "react";

export function EmptyState({ children, style, ...rest }: { children?: ReactNode } & HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      style={{
        padding: "38px 16px",
        textAlign: "center",
        color: "var(--text-4)",
        fontSize: "var(--fs-table)",
        fontStyle: "italic",
        ...style,
      }}
      {...rest}
    >
      {children}
    </div>
  );
}
