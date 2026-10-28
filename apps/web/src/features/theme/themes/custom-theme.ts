import { type AppearancePreferencePort } from "@/features/theme/application/appearancePreference";
import { customPalette } from "@/features/theme/kit/customPalette";
import { paletteThemePlugin } from "@/features/theme/themes/paletteThemePlugin";
export default function createThemePlugin(preference: AppearancePreferencePort) {
  return paletteThemePlugin(preference, "flame.builtin.custom-theme", ({ customTheme }) =>
    customPalette(customTheme),
  );
}
