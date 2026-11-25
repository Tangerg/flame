import { rgbColor } from "@/features/theme/kit/color";
import { type AppearancePreference } from "@/features/theme/kit/appearance";
import { type PaletteTheme } from "@/features/theme/kit/palette";

export const CUSTOM_THEME = "custom";

export function customPalette(colors: AppearancePreference["customTheme"]): PaletteTheme {
  const { r, g, b } = rgbColor(colors.bg)!;
  return {
    id: CUSTOM_THEME,
    label: "settings.theme.custom",
    scheme: (299 * r + 587 * g + 114 * b) / 1000 < 0.5 ? "dark" : "light",
    icon: "palette",
    order: 99,
    palette: { background: colors.bg, foreground: colors.fg },
  };
}
