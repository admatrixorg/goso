import type { HTMLAttributes } from "react";
import { cn } from "../lib/cn";

export type Status = "success" | "warning" | "error" | "info" | "default";

const statusClasses: Record<Status, string> = {
  success: "bg-success/15 text-success",
  warning: "bg-warning/15 text-warning",
  error: "bg-destructive/15 text-destructive",
  info: "bg-primary/15 text-primary",
  default: "bg-muted text-muted-foreground",
};

const dotClasses: Record<Status, string> = {
  success: "bg-success",
  warning: "bg-warning",
  error: "bg-destructive",
  info: "bg-primary",
  default: "bg-muted-foreground",
};

export function StatusBadge({
  status,
  label,
  className,
  ...rest
}: {
  status: Status;
  label: string;
} & HTMLAttributes<HTMLSpanElement>) {
  return (
    <span
      data-status={status}
      className={cn("inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium", statusClasses[status], className)}
      {...rest}
    >
      <span className={cn("h-1.5 w-1.5 rounded-full", dotClasses[status])} />
      {label}
    </span>
  );
}
