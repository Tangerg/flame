import type { AnyPlugin } from "dougong";
import { appearancePainter } from "./appearancePainter";
import customTheme from "./themes/custom-theme";
import flameDark from "./themes/flame-dark";
import flameLight from "./themes/flame-light";
import { builtinVisualStyles } from "./visualStyles";

const builtinThemes: AnyPlugin[] = [flameDark, flameLight];

export const appearancePlugins: AnyPlugin[] = [
  ...builtinThemes,
  customTheme,
  ...builtinVisualStyles,
  appearancePainter,
];
