import { type AnyPlugin } from "dougong";
import { appearancePainter } from "@/features/theme/appearancePainter";
import customTheme from "@/features/theme/themes/custom-theme";
import flameDark from "@/features/theme/themes/flame-dark";
import flameLight from "@/features/theme/themes/flame-light";
import { builtinVisualStyles } from "@/features/theme/visualStyles/index";
const builtinThemes: AnyPlugin[] = [flameDark, flameLight];

export const appearancePlugins: AnyPlugin[] = [
  ...builtinThemes,
  customTheme,
  ...builtinVisualStyles,
  appearancePainter,
];
