import type { HTMLAttributes, ReactNode } from "react";
import { Icon, type IconName } from "./Icon";
import { cn } from "../lib/cn";

export function KpiCard({
  label,
  value,
  unit,
  icon,
  note,
  noteTone,
  className,
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
    <div className={cn("rounded-lg border bg-card p-4 min-w-0", className)} {...rest}>
      <div className="flex items-start gap-2">
        <div className="flex-1 text-xs font-medium text-muted-foreground">{label}</div>
        {icon ? (
          <span className="flex text-muted-foreground">
            <Icon name={icon} size={16} />
          </span>
        ) : null}
      </div>
      <div className="mt-2 text-2xl font-semibold tracking-tight tabular-nums">
        {value}
        {unit ? <span className="ml-1 text-sm font-medium text-muted-foreground">{unit}</span> : null}
      </div>
      {note ? (
        <div
          className={cn(
            "mt-3 rounded-md px-2 py-1 text-xs",
            noteTone === "critical" ? "bg-destructive/15 text-destructive" : "bg-muted text-muted-foreground",
          )}
        >
          {note}
        </div>
      ) : null}
    </div>
  );
}
