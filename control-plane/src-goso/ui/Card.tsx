import type { HTMLAttributes, ReactNode } from "react";
import { Icon, type IconName } from "./Icon";
import { cn } from "../lib/cn";

export function Card({ children, className, style, ...rest }: { children?: ReactNode } & HTMLAttributes<HTMLDivElement>) {
  return (
    <div className={cn("rounded-lg border bg-card text-card-foreground min-w-0", className)} style={style} {...rest}>
      {children}
    </div>
  );
}

export function TableScroll({ children }: { children?: ReactNode }) {
  return (
    <div className="overflow-x-auto min-w-0 rounded-md border bg-card">
      <div className="min-w-[600px]">{children}</div>
    </div>
  );
}

export function CardHeader({
  icon,
  iconColor,
  title,
  meta,
  className,
  style,
  ...rest
}: {
  icon?: IconName;
  iconColor?: string;
  title: ReactNode;
  meta?: ReactNode;
} & HTMLAttributes<HTMLDivElement>) {
  return (
    <div className={cn("flex items-center gap-2 border-b px-4 py-3 text-sm font-semibold", className)} style={style} {...rest}>
      {icon ? (
        <span className="flex text-muted-foreground" style={iconColor ? { color: iconColor } : undefined}>
          <Icon name={icon} size={16} />
        </span>
      ) : null}
      <span>{title}</span>
      {meta ? <span className="ml-auto text-xs font-normal text-muted-foreground">{meta}</span> : null}
    </div>
  );
}
