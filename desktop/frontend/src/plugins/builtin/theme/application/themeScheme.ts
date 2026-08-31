import type { Scheme } from "@/lib/appearance";
import { COLOR_THEME } from "@/plugins/sdk/kernelPoints";
import { lookupExtensionByKey, lookupExtensionPoint } from "@/plugins/sdk/selectors/extensions";
import { systemAppearance } from "./ports/systemAppearance";
import { appearancePreferencePort } from "./ports/appearancePreference";

/**
 * Callers asking "is this light?" MUST resolve through here rather than comparing the id
 * against `"light"`: a contributed id like `"solarized-light"` mis-classifies. Unregistered
 * ids read as dark, which covers early boot and a saved id whose plugin is gone.
 */
export function resolveThemeScheme(themeId: string): Scheme {
  if (themeId === "system") return systemAppearance().scheme();
  return lookupExtensionByKey(COLOR_THEME, themeId)?.scheme ?? "dark";
}

export function isLightTheme(themeId: string): boolean {
  return resolveThemeScheme(themeId) === "light";
}

/**
 * Here rather than on the store: picking WHICH theme comes next needs the COLOR_THEME
 * registry, and a store reaching into the plugin registry is a store that knows about the
 * plugin system. The store holds the value; this decides it.
 */
export function toggleThemeScheme(): void {
  const preference = appearancePreferencePort();
  const target = resolveThemeScheme(preference.read().theme) === "dark" ? "light" : "dark";
  const next = lookupExtensionPoint(COLOR_THEME).find((spec) => spec.scheme === target);
  if (next) preference.edit().setTheme(next.id);
}
