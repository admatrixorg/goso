import { useEffect, type ReactNode } from "react";
import { cn } from "../lib/cn";

export function Dialog({
  open,
  onOpenChange,
  children,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  children: ReactNode;
}) {
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onOpenChange(false);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, onOpenChange]);

  if (!open) return null;
  return (
    <div
      className="fixed inset-0 z-[80] flex items-end justify-center bg-black/50 sm:items-center sm:p-6"
      role="presentation"
      onClick={(e) => {
        if (e.target === e.currentTarget) onOpenChange(false);
      }}
    >
      <div
        className="flex max-h-dvh w-full flex-col gap-3 bg-background p-5 max-sm:absolute max-sm:inset-0 max-sm:rounded-none sm:max-w-lg sm:rounded-lg sm:border sm:shadow-md"
        role="dialog"
        aria-modal="true"
        onClick={(e) => e.stopPropagation()}
      >
        {children}
      </div>
    </div>
  );
}

export function DialogHeader({ title, description }: { title: ReactNode; description?: ReactNode }) {
  return (
    <div>
      <h2 className="text-lg font-semibold">{title}</h2>
      {description ? <p className="mt-1 text-sm text-muted-foreground">{description}</p> : null}
    </div>
  );
}

export function DialogFooter({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={cn("mt-2 flex flex-wrap justify-end gap-2", className)}>{children}</div>;
}
