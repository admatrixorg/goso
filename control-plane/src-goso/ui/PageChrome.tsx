import type { HTMLAttributes, ReactNode } from "react";
import { PageHeader } from "./PageHeader";

export function PageChrome({
  icon: _icon,
  title,
  description,
  primary,
  refresh,
  filters,
  children,
  className,
  ...rest
}: {
  icon?: string;
  title: ReactNode;
  description?: ReactNode;
  primary?: ReactNode;
  refresh?: ReactNode;
  filters?: ReactNode;
  children?: ReactNode;
  className?: string;
} & HTMLAttributes<HTMLDivElement>) {
  void _icon;
  return (
    <div className={["p-4 sm:p-6 pb-10 space-y-4", className].filter(Boolean).join(" ")} {...rest}>
      <PageHeader
        title={title}
        description={description}
        actions={
          primary || refresh ? (
            <>
              {refresh}
              {primary}
            </>
          ) : undefined
        }
      />
      {filters ? <div className="flex flex-wrap items-center gap-2">{filters}</div> : null}
      {children}
    </div>
  );
}
