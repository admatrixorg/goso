import { cn } from "@/lib/utils";

type Status = "success" | "warning" | "error" | "info" | "default";

const statusClasses: Record<Status, string> = {
  success: "bg-success/15 text-emerald-800 dark:text-emerald-300",
  warning: "bg-warning/15 text-amber-800 dark:text-amber-300",
  error: "bg-destructive/15 text-red-800 dark:text-red-300",
  info: "bg-primary/15 text-blue-800 dark:text-blue-300",
  default: "bg-muted text-muted-foreground",
};

const dotClasses: Record<Status, string> = {
  success: "bg-success",
  warning: "bg-warning",
  error: "bg-destructive",
  info: "bg-primary",
  default: "bg-muted-foreground",
};

interface StatusBadgeProps {
  status: Status;
  label: string;
  className?: string;
}

export function StatusBadge({ status, label, className }: StatusBadgeProps) {
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium",
        statusClasses[status],
        className,
      )}
    >
      <span className={cn("h-1.5 w-1.5 rounded-full", dotClasses[status])} />
      {label}
    </span>
  );
}
