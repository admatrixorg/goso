import type { HTMLAttributes, ReactNode } from "react";
import { Icon, type IconName } from "./Icon";
import { cn } from "../lib/cn";

export function EmptyState({
  icon,
  title,
  description,
  action,
  children,
  className,
  ...rest
}: {
  icon?: IconName;
  title?: ReactNode;
  description?: ReactNode;
  action?: ReactNode;
  children?: ReactNode;
} & HTMLAttributes<HTMLDivElement>) {
  const heading = title ?? (typeof children === "string" ? children : undefined);
  return (
    <div className={cn("flex flex-col items-center justify-center py-16 text-center", className)} {...rest}>
      {icon ? (
        <div className="mb-4 rounded-full bg-muted p-3">
          <Icon name={icon} size={24} className="h-6 w-6 text-muted-foreground" />
        </div>
      ) : null}
      {heading ? <h3 className="text-sm font-medium">{heading}</h3> : null}
      {description ? <p className="mt-1 text-sm text-muted-foreground">{description}</p> : null}
      {action ? <div className="mt-4">{action}</div> : null}
      {!heading && children ? children : null}
    </div>
  );
}
