import type { HTMLAttributes } from "react";

export function Avatar({
  initials,
  color = "var(--accent)",
  size = 26,
  style,
  ...rest
}: { initials: string; color?: string; size?: number } & HTMLAttributes<HTMLSpanElement>) {
  return (
    <span
      style={{
        width: size,
        height: size,
        borderRadius: "50%",
        flex: "none",
        background: color,
        color: "#fff",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        fontFamily: "var(--font-sans)",
        fontWeight: 600,
        fontSize: size >= 34 ? 13 : size >= 26 ? 11 : 10,
        ...style,
      }}
      {...rest}
    >
      {initials}
    </span>
  );
}
