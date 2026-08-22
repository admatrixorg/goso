import type { HTMLAttributes, ReactNode } from "react";

const tones = {
  neutral: { background: "var(--surface-2)", color: "var(--text-3)" },
  accent: { background: "var(--accent-soft)", color: "var(--accent)" },
  positive: { background: "var(--green-bg)", color: "var(--green)" },
  warning: { background: "var(--warn-bg)", color: "var(--orange)" },
  critical: { background: "var(--red-bg)", color: "var(--red)" },
  solid: { background: "var(--red)", color: "#fff" },
};

export function Badge({
  tone = "neutral",
  children,
  style,
  ...rest
}: { tone?: keyof typeof tones; children?: ReactNode } & HTMLAttributes<HTMLSpanElement>) {
  return (
    <span
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: "var(--space-4)",
        padding: "var(--pad-badge)",
        borderRadius: "var(--radius-md)",
        fontFamily: "var(--font-sans)",
        fontSize: "var(--fs-badge)",
        fontWeight: 600,
        whiteSpace: "nowrap",
        ...tones[tone],
        ...style,
      }}
      {...rest}
    >
      {children}
    </span>
  );
}
