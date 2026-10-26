import { type Theme } from "@/features/theme/adapters/theme";
import { type AnyPlugin } from "dougong";
import { appearancePainter } from "@/features/theme/appearancePainter";
import customTheme from "@/features/theme/themes/custom-theme";
import flameDark from "@/features/theme/themes/flame-dark";
import flameLight from "@/features/theme/themes/flame-light";
import { builtinVisualStyles } from "@/features/theme/visualStyles/index";
export function createAppearancePlugins(theme: Theme): AnyPlugin[] {
  const preference = theme.access.preference;
  return [
    flameDark(preference),
    flameLight(preference),
    customTheme(preference),
    ...builtinVisualStyles,
    appearancePainter(theme),
  ];
}
