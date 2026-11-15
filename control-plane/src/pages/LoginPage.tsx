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

/**
 * Login gate for the Control Plane.
 *
 * The Control Plane has no server session. "Logging in" means proving the
 * admin bearer this browser will send to /api/* actually works: the page
 * probes GET /api/agents with the pasted token, and only on 200 writes it to
 * localStorage under goso_token. A 401 keeps the token out of storage, so a
 * wrong paste cannot leave the UI in a half-authenticated state.
 *
 * The token is never prefilled. It is never read back out of storage for
 * display, and it is never written to the URL, a log, or an error string.
 */
export function LoginPage({ onDone }: { onDone: () => void }) {
  const { t } = useI18n();
  const [token, setToken] = useState(emptyBrowserTokenInput);
  const [busy, setBusy] = useState(false);
  const [probe, setProbe] = useState<BrowserTokenProbe | "">("");
  const [envOwned, setEnvOwned] = useState(false);

  useEffect(() => {
    // A build-time token wins over this page; say so instead of accepting a
    // paste that would never be used.
    const hasEnv = Boolean((import.meta.env.VITE_GOSO_ADMIN_TOKEN as string) || "");
    setEnvOwned(hasEnv);
    if (hasEnv) onDone();
  }, [onDone]);

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
    <div
      style={{
        minHeight: "100vh",
        display: "grid",
        placeItems: "center",
        background: "var(--bg)",
        padding: 24,
      }}
    >
      <form
        onSubmit={(e) => {
          e.preventDefault();
          void submit();
        }}
        style={{
          width: "100%",
          maxWidth: 400,
          background: "var(--card)",
          border: "var(--border-card)",
          borderRadius: "var(--radius-card)",
          padding: 28,
        }}
      >
        <div style={{ display: "flex", alignItems: "center", gap: 10, marginBottom: 6 }}>
          <Icon name="user" size={18} />
          <div style={{ fontSize: 17, fontWeight: 600 }}>{t("login.title")}</div>
        </div>
        <div style={{ fontSize: 12.5, color: "var(--text-3)", lineHeight: 1.55, marginBottom: 20 }}>
          {t("login.desc")}
        </div>

        <label style={{ display: "block", fontSize: 12, fontWeight: 600, marginBottom: 6 }}>
          {t("login.tokenLabel")}
        </label>
        <div className="z-field">
          <input
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
            style={{ width: "100%" }}
          />
        </div>

        {probe === "unauthorized" ? (
          <div style={{ marginTop: 12, fontSize: 12.5, color: "var(--danger, #c0392b)" }}>
            {t("login.error.invalid")}
          </div>
        ) : null}
        {probe === "unreachable" ? (
          <div style={{ marginTop: 12, fontSize: 12.5, color: "var(--danger, #c0392b)" }}>
            {t("login.error.unreachable")}
          </div>
        ) : null}

        <div style={{ marginTop: 20 }}>
          <Button variant="primary" type="submit" disabled={busy || !token.trim()} style={{ width: "100%" }}>
            {busy ? t("login.submitting") : t("login.submit")}
          </Button>
        </div>
      </form>
    </div>
  );
}

/** Reports whether this browser already carries a token the UI will send. */
export function hasBrowserToken(): boolean {
  return browserTokenKind({ viteAdminToken: (import.meta.env.VITE_GOSO_ADMIN_TOKEN as string) || "" }, localStorage) === "set";
}

/** True when the build owns the token, so no paste is needed. */
export function tokenIsEnvOwned(): boolean {
  return browserTokenKind({ viteAdminToken: (import.meta.env.VITE_GOSO_ADMIN_TOKEN as string) || "" }, localStorage) === "env-owned";
}

export { classifyTokenProbeStatus };
