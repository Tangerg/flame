export function presentationTitle(
  fallback: string,
  translations: Readonly<Record<string, string>> | undefined,
  locale: string,
): string {
  if (!translations) return fallback;
  return translations[locale] ?? translations[locale.split("-")[0]!] ?? fallback;
}
