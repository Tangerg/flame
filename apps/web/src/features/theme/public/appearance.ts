import { useSyncExternalStore } from "react";
import { type AppearanceEdit, type AppearancePreference } from "@/features/theme/kit/appearance";
import { useTheme } from "@/features/theme/public/theme";
export function useAppearance<T>(select: (preference: AppearancePreference) => T): T {
  const { preference } = useTheme();
  return useSyncExternalStore(preference.subscribe, () => select(preference.read()));
}

export function useAppearanceEdit(): AppearanceEdit {
  return useTheme().preference.edit();
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
