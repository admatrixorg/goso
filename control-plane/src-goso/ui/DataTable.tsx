import type { HTMLAttributes, ReactNode, TdHTMLAttributes, ThHTMLAttributes } from "react";
import { cn } from "../lib/cn";

export function DataTable({
  children,
  minWidth = "600px",
  className,
}: {
  children: ReactNode;
  minWidth?: string;
  className?: string;
}) {
  return (
    <div className={cn("mt-4 overflow-x-auto rounded-md border", className)}>
      <table className="w-full" style={{ minWidth }}>
        {children}
      </table>
    </div>
  );
}

export function THead({ children }: { children: ReactNode }) {
  return (
    <thead>
      <tr className="border-b bg-muted/50">{children}</tr>
    </thead>
  );
}

export function Th({ className, children, ...rest }: ThHTMLAttributes<HTMLTableCellElement>) {
  return (
    <th className={cn("px-4 py-3 text-left text-sm font-medium", className)} {...rest}>
      {children}
    </th>
  );
}

export function Td({ className, children, ...rest }: TdHTMLAttributes<HTMLTableCellElement>) {
  return (
    <td className={cn("px-4 py-3 text-sm", className)} {...rest}>
      {children}
    </td>
  );
}

export function Tr({
  selected,
  className,
  children,
  ...rest
}: { selected?: boolean } & HTMLAttributes<HTMLTableRowElement>) {
  return (
    <tr
      className={cn(
        "border-b last:border-0 transition-colors hover:bg-muted/40",
        selected && "bg-accent/60",
        rest.onClick && "cursor-pointer",
        className,
      )}
      {...rest}
    >
      {children}
    </tr>
  );
}
