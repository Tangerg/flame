import i18next, { type FormatterModule } from "i18next";
import { useCallback } from "react";
import { initReactI18next, useTranslation } from "react-i18next";
import { en } from "@/lib/i18n/locales/en";
import { initialLocale, localeToBcp47, LOCALE_STORAGE_KEY, type Locale } from "./locale";
import { forgetStoredDictionary, readStoredDictionary, storeDictionary } from "./dictionaryStorage";
export { Trans } from "react-i18next";
export { browserLocale, localeToBcp47 } from "./locale";
export type { Locale } from "./locale";

const initial = initialLocale();
// Dictionaries arrive with installed locale packages after the Runtime answers. The
// active one is kept from the previous launch so startup does not flash English; it
// stays provisional until its package mounts the current release or is reconciled away.
const stored = readStoredDictionary(initial);
let provisional: Locale | undefined = stored?.locale;
const mountedSources = new Map<Locale, string>();

const numberFormats = new Map<string, Intl.NumberFormat>();

// Numbers read in the locale's digits; a caller passes a string to keep a value verbatim.
const numberFormatter: FormatterModule = {
  type: "formatter",
  init() {},
  add() {},
  addCached() {},
  format(value, _format, lng) {
    if (typeof value !== "number") return value;
    const locale = localeToBcp47(lng ?? getLocale());
    let format = numberFormats.get(locale);
    if (!format) {
      format = new Intl.NumberFormat(locale);
      numberFormats.set(locale, format);
    }
    return format.format(value);
  },
};

void i18next
  .use(numberFormatter)
  .use(initReactI18next)
  .init({
    resources: {
      en: { translation: en },
      ...(stored ? { [stored.locale]: { translation: stored.messages } } : {}),
    },
    lng: initial,
    fallbackLng: "en",
    react: { bindI18nStore: "added removed" },
    keySeparator: false,
    nsSeparator: false,
    interpolation: { escapeValue: false, alwaysFormat: true },
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
  rememberActiveDictionary(loc);
}

function rememberActiveDictionary(locale: Locale): void {
  const source = mountedSources.get(locale);
  if (source === undefined) {
    forgetStoredDictionary();
    return;
  }
  storeDictionary({
    locale,
    source,
    messages: i18next.getResourceBundle(locale, "translation") as Record<string, string>,
  });
}

type CatalogKey = keyof typeof en;
type PluralForm = "zero" | "one" | "two" | "few" | "many" | "other";
export type MessageKey = CatalogKey extends infer K
  ? K extends `${infer Base}_${PluralForm}`
    ? Base
    : K
  : never;

export function t(key: MessageKey, params?: Record<string, string | number>): string {
  return params === undefined ? i18next.t(key) : i18next.t(key, params);
}

export function isMessageKey(key: string): key is MessageKey {
  return Object.hasOwn(en, key);
}

// Built-in contributions name a catalog message; installed packages supply text
// they already localized.
export type Label = MessageKey | { readonly text: string };

export function labelText(label: Label, translate: Translate = t): string {
  return typeof label === "string" ? translate(label) : label.text;
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

export function mountLocaleBundle(
  locale: Locale,
  source: string,
  dict: Record<string, string>,
): () => void {
  if (locale === provisional) {
    provisional = undefined;
    i18next.removeResourceBundle(locale, "translation");
  }
  if (i18next.hasResourceBundle(locale, "translation"))
    throw new Error(`locale "${locale}" already has a dictionary`);
  i18next.addResourceBundle(locale, "translation", dict);
  mountedSources.set(locale, source);
  if (locale === getLocale()) rememberActiveDictionary(locale);
  return () => {
    mountedSources.delete(locale);
    i18next.removeResourceBundle(locale, "translation");
  };
}

export function retireProvisionalLocale(available: ReadonlySet<Locale>): void {
  if (provisional === undefined || available.has(provisional)) return;
  i18next.removeResourceBundle(provisional, "translation");
  provisional = undefined;
  forgetStoredDictionary();
}
