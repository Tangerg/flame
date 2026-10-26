import { type AppearancePreferencePort } from "@/features/theme/application/appearancePreference";
import { paletteThemePlugin } from "@/features/theme/themes/paletteThemePlugin";
export default function createThemePlugin(preference: AppearancePreferencePort) {
  return paletteThemePlugin(preference, "flame.builtin.color-theme-dark", () => ({
    id: "dark",
    label: "Dark",
    scheme: "dark",
    order: 0,
    palette: {},
  }));
}
