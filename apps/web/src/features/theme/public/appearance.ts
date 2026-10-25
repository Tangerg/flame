import { type AppearanceEdit, type AppearancePreference } from "@/features/theme/kit/appearance";
import { appearancePreferencePort } from "@/features/theme/application/ports/appearancePreference";
export function useAppearance<T>(select: (preference: AppearancePreference) => T): T {
  return appearancePreferencePort().use(select);
}

export function editAppearance(): AppearanceEdit {
  return appearancePreferencePort().edit();
}
export {
  DEFAULT_ACCENT,
  PAPER_TEXTURES,
  UI_DENSITY_MODES,
  type PaperTexture,
  type UiDensity,
} from "@/features/theme/kit/appearance";
export { retainThemeSelection } from "@/features/theme/application/themeScheme";
export { contributePaletteTheme } from "@/features/theme/application/paletteTheme";
