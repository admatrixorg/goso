import type { HTMLAttributes, ReactNode } from "react";
import type { IconName } from "./Icon";
import { SectionHeader } from "./SectionHeader";

/** Shared CORE page shell: title, one primary CTA slot, refresh, optional filters. */
export function PageChrome({
  icon,
  title,
  description,
  primary,
  refresh,
  filters,
  children,
  style,
  ...rest
}: {
  icon?: IconName;
  title: ReactNode;
  description?: ReactNode;
  primary?: ReactNode;
  refresh?: ReactNode;
  filters?: ReactNode;
  children?: ReactNode;
} & HTMLAttributes<HTMLDivElement>) {
  return (
    <div style={{ padding: "14px 22px 40px", display: "flex", flexDirection: "column", gap: 14, ...style }} {...rest}>
      <SectionHeader
        icon={icon}
        title={title}
        description={description}
        actions={
          <div style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "center" }}>
            {primary}
            {refresh}
          </div>
        }
      />
      {filters ? <div style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "center" }}>{filters}</div> : null}
      {children}
    </div>
  );
}
