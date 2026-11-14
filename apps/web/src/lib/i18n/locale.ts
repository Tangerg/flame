export type Locale = string;

export const LOCALE_STORAGE_KEY = "flame.locale";

export function initialLocale(): Locale {
  try {
    const stored = localStorage.getItem(LOCALE_STORAGE_KEY);
    if (stored) return stored;
  } catch {}
  return browserLocale(typeof navigator === "undefined" ? "" : navigator.language);
}

export function browserLocale(language: string): Locale {
  if (!language) return "en";
  try {
    const locale = new Intl.Locale(language);
    if (locale.language === "zh") return locale.maximize().script === "Hant" ? "zh-TW" : "zh";
    return locale.language;
  } catch {
    return "en";
  }
}

export function localeToBcp47(locale: Locale): string {
  return locale === "zh" ? "zh-CN" : locale;
}
