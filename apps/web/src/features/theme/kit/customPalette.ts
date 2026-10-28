import { colord } from "colord";
import { type AppearancePreference } from "@/features/theme/kit/appearance";
import { type PaletteTheme } from "@/features/theme/kit/palette";

export function customPalette(colors: AppearancePreference["customTheme"]): PaletteTheme {
  return {
    id: "custom",
    label: "Custom",
    scheme: colord(colors.bg).isDark() ? "dark" : "light",
    icon: "palette",
    order: 99,
    palette: { background: colors.bg, foreground: colors.fg },
  };
}
