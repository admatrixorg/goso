import { Icon } from "../../ui/Icon";

export function LoginLayout({
  children,
  subtitle,
  dark,
  onToggleTheme,
  themeLabel,
}: {
  children: React.ReactNode;
  subtitle?: string;
  dark: boolean;
  onToggleTheme: () => void;
  themeLabel: string;
}) {
  return (
    <div className="relative flex min-h-dvh items-center justify-center bg-background px-4">
      <button
        type="button"
        onClick={onToggleTheme}
        className="absolute top-4 right-4 cursor-pointer rounded-md border bg-card p-2 text-muted-foreground shadow-sm transition-colors hover:bg-accent hover:text-accent-foreground"
        title={themeLabel}
        aria-label={themeLabel}
      >
        {dark ? <Icon name="sun" size={16} className="h-4 w-4" /> : <Icon name="moon" size={16} className="h-4 w-4" />}
      </button>
      <div className="w-full max-w-sm space-y-6 rounded-lg border bg-card p-6 shadow-sm sm:p-8">
        <div className="text-center">
          <div className="mx-auto mb-3 h-24 w-24" aria-hidden="true">
            <img src="/admatrix-mark-transparent-v3.png" alt="" className="h-full w-full object-contain" />
          </div>
          <h1 className="bg-gradient-to-r from-blue-700 via-blue-500 to-cyan-400 bg-clip-text text-3xl font-bold tracking-tight text-transparent">
            GOSO
          </h1>
          {subtitle ? <p className="mt-2 text-sm text-muted-foreground">{subtitle}</p> : null}
        </div>
        {children}
      </div>
    </div>
  );
}
