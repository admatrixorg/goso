import { useEffect, useState } from "react";
import {
  BROWSER_TOKEN_STORAGE_KEY,
  browserTokenKind,
  classifyTokenProbeStatus,
  emptyBrowserTokenInput,
  probeBrowserToken,
  type BrowserTokenProbe,
} from "../api/browser-token";
import { useI18n } from "../i18n";
import { Button } from "../ui/Button";
import { Icon } from "../ui/Icon";
import { LoginLayout } from "./login/login-layout";
import { LoginTabs, type LoginMode } from "./login/login-tabs";

export function LoginPage({ onDone }: { onDone: () => void }) {
  const { t } = useI18n();
  const [mode, setMode] = useState<LoginMode>("token");
  const [token, setToken] = useState(emptyBrowserTokenInput);
  const [busy, setBusy] = useState(false);
  const [probe, setProbe] = useState<BrowserTokenProbe | "">("");
  const [envOwned, setEnvOwned] = useState(false);
  const [dark, setDark] = useState(
    () => document.documentElement.classList.contains("dark") || document.body.classList.contains("dark"),
  );

  useEffect(() => {
    const hasEnv = Boolean((import.meta.env.VITE_GOSO_ADMIN_TOKEN as string) || "");
    setEnvOwned(hasEnv);
    if (hasEnv) onDone();
  }, [onDone]);

  useEffect(() => {
    document.documentElement.classList.toggle("dark", dark);
    document.body.classList.toggle("dark", dark);
  }, [dark]);

  async function submit() {
    const value = token.trim();
    if (!value || busy) return;
    setBusy(true);
    setProbe("");
    try {
      const result = await probeBrowserToken(fetch, "", value);
      setProbe(result);
      if (result === "accepted") {
        try {
          localStorage.setItem(BROWSER_TOKEN_STORAGE_KEY, value);
        } catch {
          setProbe("unreachable");
          return;
        }
        setToken("");
        onDone();
        return;
      }
    } finally {
      setBusy(false);
    }
  }

  if (envOwned) return null;

  return (
    <LoginLayout
      subtitle={t("login.desc")}
      dark={dark}
      onToggleTheme={() => setDark((d) => !d)}
      themeLabel={dark ? t("chrome.light") : t("chrome.dark")}
    >
      <LoginTabs
        mode={mode}
        onModeChange={setMode}
        labels={{ email: t("login.tabs.email"), token: t("login.tabs.token"), pairing: t("login.tabs.pairing") }}
      />
      {mode === "token" ? (
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            void submit();
          }}
        >
          <div className="space-y-2">
            <label className="text-sm font-medium" htmlFor="gatewayToken">
              {t("login.tokenLabel")}
            </label>
            <input
              id="gatewayToken"
              type="password"
              autoFocus
              autoComplete="off"
              spellCheck={false}
              value={token}
              disabled={busy}
              placeholder={t("login.tokenPlaceholder")}
              onChange={(e) => {
                setToken(e.target.value);
                if (probe) setProbe("");
              }}
              className="border-input h-9 w-full rounded-md border bg-transparent px-3 text-base shadow-xs outline-none md:text-sm placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-ring/50 focus-visible:ring-[3px]"
            />
          </div>
          {probe === "unauthorized" ? (
            <div className="flex items-start gap-2 rounded-md border border-destructive/50 bg-destructive/10 px-3 py-2 text-sm text-destructive">
              <Icon name="flag" size={16} className="mt-0.5 h-4 w-4 shrink-0" />
              <span>{t("login.error.invalid")}</span>
            </div>
          ) : null}
          {probe === "unreachable" ? (
            <div className="flex items-start gap-2 rounded-md border border-destructive/50 bg-destructive/10 px-3 py-2 text-sm text-destructive">
              <Icon name="flag" size={16} className="mt-0.5 h-4 w-4 shrink-0" />
              <span>{t("login.error.unreachable")}</span>
            </div>
          ) : null}
          <Button variant="default" type="submit" disabled={busy || !token.trim()} className="h-9 w-full shadow">
            {busy ? t("login.submitting") : t("login.submit")}
          </Button>
        </form>
      ) : (
        <p className="text-center text-sm text-muted-foreground">{t("login.tabUnavailable")}</p>
      )}
    </LoginLayout>
  );
}

export function hasBrowserToken(): boolean {
  return browserTokenKind({ viteAdminToken: (import.meta.env.VITE_GOSO_ADMIN_TOKEN as string) || "" }, localStorage) === "set";
}

export function tokenIsEnvOwned(): boolean {
  return browserTokenKind({ viteAdminToken: (import.meta.env.VITE_GOSO_ADMIN_TOKEN as string) || "" }, localStorage) === "env-owned";
}

export { classifyTokenProbeStatus };
