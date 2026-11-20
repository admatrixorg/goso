import { cn } from "../../lib/cn";

export type LoginMode = "email" | "token" | "pairing";

export function LoginTabs({
  mode,
  onModeChange,
  labels,
}: {
  mode: LoginMode;
  onModeChange: (mode: LoginMode) => void;
  labels: { email: string; token: string; pairing: string };
}) {
  function Tab({ id, label }: { id: LoginMode; label: string }) {
    return (
      <button
        type="button"
        onClick={() => onModeChange(id)}
        className={cn(
          "flex-1 rounded-sm px-3 py-1.5 text-sm font-medium transition-colors",
          mode === id ? "bg-background text-foreground shadow-sm" : "text-muted-foreground hover:text-foreground",
        )}
      >
        {label}
      </button>
    );
  }
  return (
    <div className="flex rounded-md border bg-muted p-1">
      <Tab id="email" label={labels.email} />
      <Tab id="token" label={labels.token} />
      <Tab id="pairing" label={labels.pairing} />
    </div>
  );
}
