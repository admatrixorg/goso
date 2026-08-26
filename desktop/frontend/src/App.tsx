import { useEffect, useState } from "react";
import ControlPlane from "../../../control-plane/src/App";

/** Wails bind target (generated under frontend/wailsjs, gitignored). */
type WailsGo = {
  go?: { main?: { App?: { LocalToken?: () => Promise<string> } } };
};

async function injectLocalToken(): Promise<void> {
  try {
    const fn = (window as WailsGo).go?.main?.App?.LocalToken;
    if (typeof fn !== "function") return;
    const token = await fn();
    if (token) localStorage.setItem("goso_token", token);
  } catch {
    // Vite-only / tests: gateway may be GOSO_DEV_MODE.
  }
}

export default function App() {
  const [ready, setReady] = useState(false);
  useEffect(() => {
    let cancelled = false;
    void injectLocalToken().finally(() => {
      if (!cancelled) setReady(true);
    });
    return () => {
      cancelled = true;
    };
  }, []);
  if (!ready) {
    return (
      <div
        style={{
          minHeight: "100vh",
          background: "var(--bg)",
          color: "var(--text-3)",
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          fontSize: 14,
        }}
      >
        GOSO
      </div>
    );
  }
  return <ControlPlane />;
}
