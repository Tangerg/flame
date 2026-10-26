import { type Scheme } from "@/lib/appearance";
import { COLOR_THEME } from "@/features/theme/public/extensionPoints";
import { lookupExtensionByKey, lookupExtensionPoint } from "@/extensions/selectors/extensions";
import { type SystemAppearance } from "@/features/theme/application/systemAppearance";
import { type ThemeAccess } from "@/features/theme/application/themeAccess";
import { type AppearancePreferencePort } from "@/features/theme/application/appearancePreference";
export function resolveThemeScheme(system: SystemAppearance, themeId: string): Scheme {
  if (themeId === "system") return system.scheme();
  return lookupExtensionByKey(COLOR_THEME, themeId)?.scheme ?? system.scheme();
}

export function retainThemeSelection(
  preference: AppearancePreferencePort,
  available: readonly string[],
  sourcePrefix: string,
): void {
  const current = preference.read().theme;
  if (current.startsWith(sourcePrefix) && !available.includes(current))
    preference.edit().setTheme("system");
}

export function isLightTheme(theme: ThemeAccess, themeId: string): boolean {
  return resolveThemeScheme(theme.system, themeId) === "light";
}

export function toggleThemeScheme(theme: ThemeAccess): void {
  const { preference, system } = theme;
  const target = resolveThemeScheme(system, preference.read().theme) === "dark" ? "light" : "dark";
  const next = lookupExtensionPoint(COLOR_THEME).find((spec) => spec.scheme === target);
  if (next) preference.edit().setTheme(next.id);
}
