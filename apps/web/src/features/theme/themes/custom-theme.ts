import { type AppearancePreferencePort } from "@/features/theme/application/appearancePreference";
import { colord } from "colord";
import { paletteThemePlugin } from "@/features/theme/themes/paletteThemePlugin";
export default function createThemePlugin(preference: AppearancePreferencePort) {
  return paletteThemePlugin(preference, "flame.builtin.custom-theme", ({ customTheme }) => ({
    id: "custom",
    label: "Custom",
    scheme: colord(customTheme.bg).isDark() ? "dark" : "light",
    icon: "palette",
    order: 99,
    palette: { background: customTheme.bg, foreground: customTheme.fg },
  }));
}
