import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import { I18nProvider } from "./i18n";
import "./styles/index.css";

const LOADER_MIN_MS = 800;
const loaderStart = performance.now();

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <I18nProvider>
      <App />
    </I18nProvider>
  </React.StrictMode>,
);

const ric = window.requestIdleCallback ?? ((cb: () => void) => setTimeout(cb, 1));
ric(() => {
  const elapsed = performance.now() - loaderStart;
  const delay = Math.max(0, LOADER_MIN_MS - elapsed);
  setTimeout(() => {
    const loader = document.getElementById("app-loader");
    if (loader) {
      loader.classList.add("fade-out");
      setTimeout(() => loader.remove(), 300);
    }
  }, delay);
});
