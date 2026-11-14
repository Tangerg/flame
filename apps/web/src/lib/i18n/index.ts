import i18next from "i18next";
import { useCallback } from "react";
import { initReactI18next, useTranslation } from "react-i18next";
import { en } from "@/lib/i18n/locales/en";
import { initialLocale, localeToBcp47, LOCALE_STORAGE_KEY, type Locale } from "./locale";
export { Trans } from "react-i18next";
export { browserLocale, localeToBcp47 } from "./locale";
export type { Locale } from "./locale";

const initial = initialLocale();

void i18next.use(initReactI18next).init({
  resources: { en: { translation: en } },
  lng: initial,
  fallbackLng: "en",
  react: { bindI18nStore: "added removed" },
  keySeparator: false,
  nsSeparator: false,
  interpolation: { escapeValue: false },
  returnNull: false,
});

function syncHtmlLang(loc: Locale): void {
  if (typeof document === "undefined") return;
  document.documentElement.lang = localeToBcp47(loc);
}

syncHtmlLang(initial);

function getLocale(): Locale {
  return i18next.language ?? i18next.resolvedLanguage ?? "en";
}

export function activeLocale(): Locale {
  return getLocale();
}

export function setLocale(loc: Locale): void {
  if (loc === getLocale()) return;
  void i18next.changeLanguage(loc);
  try {
    localStorage.setItem(LOCALE_STORAGE_KEY, loc);
  } catch {}
  syncHtmlLang(loc);
}

export function t(key: string, params?: Record<string, string | number>): string {
  return params === undefined ? i18next.t(key) : i18next.t(key, params);
}

export type Translate = typeof t;

export function useLocale(): Locale {
  const { i18n } = useTranslation();
  return i18n.language ?? i18n.resolvedLanguage ?? "en";
}

export function useT(): typeof t {
  const { t: translate } = useTranslation();
  return useCallback<typeof t>(
    (key, params) => (params === undefined ? translate(key) : translate(key, params)),
    [translate],
  );
}

export function mountLocaleBundle(locale: string, dict: Record<string, string>): () => void {
  if (i18next.hasResourceBundle(locale, "translation"))
    throw new Error(`locale "${locale}" already has a dictionary`);
  i18next.addResourceBundle(locale, "translation", dict);
  return () => i18next.removeResourceBundle(locale, "translation");
}
