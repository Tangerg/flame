import { type Scheme } from "@/lib/appearance";
import { COLOR_THEME } from "@/features/theme/public/extensionPoints";
import { lookupExtensionByKey, lookupExtensionPoint } from "@/extensions/selectors/extensions";
import { systemAppearance } from "@/features/theme/application/ports/systemAppearance";
import { appearancePreferencePort } from "@/features/theme/application/ports/appearancePreference";
export function resolveThemeScheme(themeId: string): Scheme {
  if (themeId === "system") return systemAppearance().scheme();
  return lookupExtensionByKey(COLOR_THEME, themeId)?.scheme ?? systemAppearance().scheme();
}

export function retainThemeSelection(available: readonly string[], sourcePrefix: string): void {
  const preference = appearancePreferencePort();
  const current = preference.read().theme;
  if (current.startsWith(sourcePrefix) && !available.includes(current))
    preference.edit().setTheme("system");
}

export function isLightTheme(themeId: string): boolean {
  return resolveThemeScheme(themeId) === "light";
}

export function toggleThemeScheme(): void {
  const preference = appearancePreferencePort();
  const target = resolveThemeScheme(preference.read().theme) === "dark" ? "light" : "dark";
  const next = lookupExtensionPoint(COLOR_THEME).find((spec) => spec.scheme === target);
  if (next) preference.edit().setTheme(next.id);
}
