import type { ButtonHTMLAttributes, ReactNode } from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "../lib/cn";
import { Icon, type IconName } from "./Icon";

const buttonVariants = cva(
  "inline-flex cursor-pointer items-center justify-center gap-2 whitespace-nowrap rounded-md text-sm font-medium transition-colors disabled:pointer-events-none disabled:opacity-50 [&_svg]:pointer-events-none [&_svg:not([class*='size-'])]:size-4 shrink-0 [&_svg]:shrink-0 outline-none focus-visible:border-ring focus-visible:ring-ring/50 focus-visible:ring-[3px]",
  {
    variants: {
      variant: {
        default: "bg-primary text-primary-foreground hover:bg-primary/90",
        destructive: "bg-destructive text-white hover:bg-destructive/90 dark:bg-destructive/60",
        outline: "border bg-background shadow-xs hover:bg-accent hover:text-accent-foreground dark:bg-input/30 dark:border-input",
        secondary: "bg-secondary text-secondary-foreground hover:bg-secondary/80",
        ghost: "hover:bg-accent hover:text-accent-foreground dark:hover:bg-accent/50",
        link: "text-primary underline-offset-4 hover:underline",
      },
      size: {
        default: "h-9 px-4 py-2 has-[>svg]:px-3",
        xs: "h-6 gap-1 rounded-md px-2 text-xs has-[>svg]:px-1.5",
        sm: "h-8 rounded-md gap-1.5 px-3 has-[>svg]:px-2.5",
        lg: "h-10 rounded-md px-6 has-[>svg]:px-4",
        icon: "size-9",
        "icon-xs": "size-6 rounded-md",
        "icon-sm": "size-8",
        "icon-lg": "size-10",
      },
    },
    defaultVariants: { variant: "default", size: "default" },
  },
);

const ALIAS: Record<string, "default" | "outline"> = {
  primary: "default",
  accent: "default",
  quiet: "outline",
};

export function Button({
  variant = "outline",
  size = "default",
  icon,
  iconGesture,
  children,
  className,
  type = "button",
  ...rest
}: {
  variant?: "default" | "destructive" | "outline" | "secondary" | "ghost" | "link" | "primary" | "quiet" | "accent";
  size?: VariantProps<typeof buttonVariants>["size"];
  icon?: IconName;
  iconGesture?: boolean;
  children?: ReactNode;
} & ButtonHTMLAttributes<HTMLButtonElement>) {
  const resolved = (ALIAS[variant] ?? variant) as VariantProps<typeof buttonVariants>["variant"];
  return (
    <button
      type={type}
      data-slot="button"
      data-variant={resolved}
      data-size={size}
      data-ig={iconGesture ? "refresh" : undefined}
      className={cn(buttonVariants({ variant: resolved, size }), className)}
      {...rest}
    >
      {icon ? <Icon name={icon} size={size === "xs" || size === "icon-xs" ? 12 : 16} /> : null}
      {children}
    </button>
  );
}
