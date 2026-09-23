import type { Scheme } from "@/lib/appearance";
import { COLOR_THEME } from "@/plugins/sdk/kernelPoints";
import { lookupExtensionByKey, lookupExtensionPoint } from "@/plugins/sdk/selectors/extensions";
import { systemAppearance } from "./ports/systemAppearance";
import { appearancePreferencePort } from "./ports/appearancePreference";

export function resolveThemeScheme(themeId: string): Scheme {
  if (themeId === "system") return systemAppearance().scheme();
  return lookupExtensionByKey(COLOR_THEME, themeId)?.scheme ?? "dark";
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
