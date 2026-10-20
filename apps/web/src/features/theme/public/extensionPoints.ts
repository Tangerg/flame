import {
  type ColorThemeSpec,
  type AccentSpec,
  type VisualStyleSpec,
} from "@/features/theme/public/types";
import { defineExtensionPoint } from "@/extensions/contracts";
export const COLOR_THEME = defineExtensionPoint<ColorThemeSpec>({
  id: "flame.colorTheme",
  keying: "single",
});

export const ACCENT = defineExtensionPoint<AccentSpec>({
  id: "flame.accent",
  keying: "single",
});

export const VISUAL_STYLE = defineExtensionPoint<VisualStyleSpec>({
  id: "flame.visualStyle",
  keying: "single",
});
