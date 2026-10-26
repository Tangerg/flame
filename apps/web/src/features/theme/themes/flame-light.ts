import { type AppearancePreferencePort } from "@/features/theme/application/appearancePreference";
import { paletteThemePlugin } from "@/features/theme/themes/paletteThemePlugin";
export default function createThemePlugin(preference: AppearancePreferencePort) {
  return paletteThemePlugin(preference, "flame.builtin.color-theme-light", () => ({
    id: "light",
    label: "Light",
    scheme: "light",
    order: 0,
    palette: {},
  }));
}
