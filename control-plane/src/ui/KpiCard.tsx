import type { HTMLAttributes, ReactNode } from "react";
import { Icon, type IconName } from "./Icon";

export function KpiCard({
  label,
  value,
  unit,
  icon,
  tint = "var(--accent)",
  tintBg = "var(--accent-soft)",
  note,
  noteTone,
  style,
  ...rest
}: {
  label: ReactNode;
  value: ReactNode;
  unit?: ReactNode;
  icon?: IconName;
  tint?: string;
  tintBg?: string;
  note?: ReactNode;
  noteTone?: "neutral" | "critical";
} & HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      style={{
        background: "var(--card)",
        border: "var(--border-card)",
        borderRadius: "var(--radius-card)",
        padding: "14px 16px",
        minWidth: 0,
        ...style,
      }}
      {...rest}
    >
      <div style={{ display: "flex", alignItems: "flex-start", gap: "var(--space-8)" }}>
        <div style={{ flex: 1, fontSize: "var(--fs-table)", color: "var(--text-2)" }}>{label}</div>
        {icon ? (
          <span
            style={{
              width: "var(--size-icon-chip)",
              height: "var(--size-icon-chip)",
              flex: "none",
              borderRadius: "var(--radius-lg)",
              background: tintBg,
              color: tint,
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
            }}
          >
            <Icon name={icon} size={14} />
          </span>
        ) : null}
      </div>
      <div
        style={{
          fontSize: "var(--fs-metric-lg)",
          fontWeight: 700,
          marginTop: "var(--space-4)",
          fontVariantNumeric: "tabular-nums",
          letterSpacing: "var(--ls-metric)",
        }}
      >
        {value}
        {unit ? (
          <span style={{ fontSize: "var(--fs-meta)", fontWeight: 500, color: "var(--text-3)", letterSpacing: 0 }}> {unit}</span>
        ) : null}
      </div>
      {note ? (
        <div
          style={{
            marginTop: "var(--space-7)",
            borderRadius: "var(--radius-md)",
            padding: "4px 9px",
            fontSize: "var(--fs-micro)",
            background: noteTone === "critical" ? "var(--red-bg)" : "var(--surface-2)",
            color: noteTone === "critical" ? "var(--red)" : "var(--text-3)",
          }}
        >
          {note}
        </div>
      ) : null}
    </div>
  );
}
