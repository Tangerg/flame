import i18next from "i18next";
import { initReactI18next, useTranslation } from "react-i18next";

/** A translated sentence containing markup. Splitting one around JSX is untranslatable. */
export { Trans } from "react-i18next";
import { en } from "@/lib/i18n/locales/en";

export type Locale = string;

const STORAGE_KEY = "flame.locale";

function detectInitial(): Locale {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored) return stored;
  } catch {
    /* ignore */
  }
  const nav = typeof navigator !== "undefined" ? navigator.language : "";
  const low = nav.toLowerCase();
  if (low.startsWith("zh")) {
    return low.includes("tw") || low.includes("hk") || low.includes("mo") ? "zh-TW" : "zh";
  }
  return low.split("-")[0] || "en";
}

const initial = detectInitial();

void i18next.use(initReactI18next).init({
  resources: { en: { translation: en } },
  lng: initial,
  fallbackLng: "en",
  // Keys are dotted LITERALS, not nested paths.
  keySeparator: false,
  nsSeparator: false,
  interpolation: { escapeValue: false },
  returnNull: false,
});

function syncHtmlLang(loc: Locale): void {
  if (typeof document === "undefined") return;
  // Only "zh" needs an explicit region; every other locale equals its lang value.
  document.documentElement.lang = loc === "zh" ? "zh-CN" : loc;
}
syncHtmlLang(initial);

function getLocale(): Locale {
  // `language` is the requested identity; `resolvedLanguage` may be the English fallback
  // while that locale's lazy plugin has not loaded. Reading the fallback makes cold-start
  // setup believe English was selected, so it never loads the requested dictionary.
  return i18next.language ?? i18next.resolvedLanguage ?? "en";
}

export function activeLocale(): Locale {
  return getLocale();
}

export function setLocale(loc: Locale): void {
  if (loc === getLocale()) return;
  void i18next.changeLanguage(loc);
  try {
    localStorage.setItem(STORAGE_KEY, loc);
  } catch {
    /* ignore */
  }
  syncHtmlLang(loc);
}

export function t(key: string, params?: Record<string, string | number>): string {
  return i18next.t(key, params) as string;
}

export type Translate = typeof t;

export function useLocale(): Locale {
  const { i18n } = useTranslation();
  return i18n.language ?? i18n.resolvedLanguage ?? "en";
}

/** Stable across renders until the language changes — safe in `useMemo` / `useCallback` deps. */
export function useT(): typeof t {
  useTranslation();
  return t;
}

/** i18next has no per-key removal, so a plugin unload does NOT roll its bundle back. */
export function addLocaleBundle(locale: string, dict: Record<string, string>): void {
  i18next.addResourceBundle(locale, "translation", dict, true, true);
}
