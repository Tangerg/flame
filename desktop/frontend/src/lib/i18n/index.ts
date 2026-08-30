// The kernel ships ONLY the English bundle; every other language is a built-in plugin that
// registers itself at setup, and the picker reads the plugin registry rather than a
// hardcoded array here.
//
// `Locale` stays `string` because selection and browser preference are runtime values.

import i18next from "i18next";
import { initReactI18next, useTranslation } from "react-i18next";

/**
 * A translated sentence that contains markup — `<code>`, `<strong>`, a link.
 *
 * Re-exported here so the whole app has one i18n import. The alternative that
 * had grown instead was splitting such a sentence into fragments around the JSX
 * ("… containing", "into", "and restarting the app"), which cannot be reordered
 * by a translator and so is not translatable at all.
 */
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
  // The primary subtag: i18next falls back to English if that plugin has not registered.
  return low.split("-")[0] || "en";
}

const initial = detectInitial();

void i18next.use(initReactI18next).init({
  resources: { en: { translation: en } },
  lng: initial,
  fallbackLng: "en",
  // Keys are dotted LITERALS ("sidebar.action.newSession"), not nested paths.
  keySeparator: false,
  nsSeparator: false,
  interpolation: { escapeValue: false },
  returnNull: false,
});

// `<html lang>` drives a11y, font selection and any Intl API that reads it.
function syncHtmlLang(loc: Locale): void {
  if (typeof document === "undefined") return;
  // Only "zh" needs the explicit region; every other locale already equals its lang value.
  document.documentElement.lang = loc === "zh" ? "zh-CN" : loc;
}
syncHtmlLang(initial);

function getLocale(): Locale {
  // `language` is the requested identity; `resolvedLanguage` may be the English fallback
  // while that locale's lazy plugin has not loaded. Reading the fallback makes cold-start
  // setup believe English was selected, so it never loads the requested dictionary.
  return i18next.language ?? i18next.resolvedLanguage ?? "en";
}

/** For reads OUTSIDE React (plugin setup, bootstrap). */
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

/**
 * `typeof t` because that is exactly what the contribution factories are handed; a caller
 * that only reads keys still satisfies it, since fewer parameters is assignable.
 */
export type Translate = typeof t;

export function useLocale(): Locale {
  const { i18n } = useTranslation();
  return i18n.language ?? i18n.resolvedLanguage ?? "en";
}

/** Hook returning a translate fn bound to the live locale. The returned
 *  reference is stable across renders (until the language changes) so it's
 *  safe to use in `useMemo` / `useCallback` deps. */
export function useT(): typeof t {
  // Subscribe for re-renders on language change; the module-level `t`
  // reads i18next live so it always sees the new locale.
  useTranslation();
  return t;
}

/**
 * i18next has no public per-key removal, so a plugin unload does NOT roll its bundle back.
 * Harmless in practice: the keys are unreferenced once the plugin's UI is gone, and a
 * same-name reload overwrites cleanly.
 */
export function addLocaleBundle(locale: string, dict: Record<string, string>): void {
  i18next.addResourceBundle(locale, "translation", dict, true, true);
}
