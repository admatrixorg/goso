import { useTranslation } from "react-i18next";

export function SetupLayout({ children }: { children: React.ReactNode }) {
  const { t } = useTranslation("setup");

  return (
    <div className="flex min-h-dvh items-start justify-center bg-background px-4 py-8 sm:items-center">
      <div className="w-full max-w-2xl space-y-6 overflow-y-auto max-h-dvh sm:max-h-none">
        <div className="text-center">
          <div className="mx-auto mb-4 h-20 w-20" aria-label="Admatrix">
            <img src="/admatrix-mark-transparent-v3.png" alt="" className="h-full w-full object-contain" />
          </div>
          <h1 className="bg-gradient-to-r from-blue-700 via-blue-500 to-cyan-400 bg-clip-text text-4xl font-bold tracking-tight text-transparent">Admatrix Setup</h1>
          <p className="mt-2 text-sm text-muted-foreground">
            {t("layout.subtitle", "Let's get your gateway up and running")}
          </p>
        </div>
        {children}
      </div>
    </div>
  );
}
