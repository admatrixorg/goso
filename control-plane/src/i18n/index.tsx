import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { en } from "./en";
import { vi, type MsgKey } from "./vi";

export type Locale = "vi" | "en";
export type { MsgKey };

export const LANG_KEY = "goso_lang";

const dicts: Record<Locale, Record<MsgKey, string>> = { vi, en };

export function readLocale(): Locale {
  try {
    const v = localStorage.getItem(LANG_KEY);
    if (v === "en" || v === "vi") return v;
  } catch {
    // private mode / SSR
  }
  return "vi";
}

export function interpolate(s: string, vars?: Record<string, string | number>): string {
  if (!vars) return s;
  return s.replace(/\{(\w+)\}/g, (_, k: string) => (vars[k] != null ? String(vars[k]) : `{${k}}`));
}

export function translate(locale: Locale, key: MsgKey, vars?: Record<string, string | number>): string {
  const pack = dicts[locale] ?? vi;
  return interpolate(pack[key] ?? vi[key] ?? key, vars);
}

type I18nValue = {
  locale: Locale;
  setLocale: (next: Locale) => void;
  t: (key: MsgKey, vars?: Record<string, string | number>) => string;
};

const I18nContext = createContext<I18nValue | null>(null);

export function I18nProvider({ children }: { children: ReactNode }) {
  const [locale, setLocaleState] = useState<Locale>(readLocale);

  const setLocale = useCallback((next: Locale) => {
    const loc: Locale = next === "en" ? "en" : "vi";
    try {
      localStorage.setItem(LANG_KEY, loc);
    } catch {
      // ignore quota
    }
    setLocaleState(loc);
  }, []);

  useEffect(() => {
    document.documentElement.lang = locale;
  }, [locale]);

  const t = useCallback((key: MsgKey, vars?: Record<string, string | number>) => translate(locale, key, vars), [locale]);

  const value = useMemo(() => ({ locale, setLocale, t }), [locale, setLocale, t]);
  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useI18n(): I18nValue {
  const ctx = useContext(I18nContext);
  if (!ctx) throw new Error("useI18n must be used within I18nProvider");
  return ctx;
}
