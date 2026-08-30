// Adding a theme is a file under `themes/` plus an entry in the array below: the manifest
// pulls in this one pack and never names an individual theme.

import type { AnyPlugin } from "dougong";
import { appearancePainter } from "./appearancePainter";
import customTheme from "./themes/custom-theme";
import atomOneDark from "./themes/atom-one-dark";
import atomOneLight from "./themes/atom-one-light";
import catppuccinLatte from "./themes/catppuccin-latte";
import catppuccinMocha from "./themes/catppuccin-mocha";
import flameDark from "./themes/flame-dark";
import flameLight from "./themes/flame-light";
import solarizedDark from "./themes/solarized-dark";
import solarizedLight from "./themes/solarized-light";
import tokyoNightLight from "./themes/tokyo-night-light";
import tokyoNightStorm from "./themes/tokyo-night-storm";
import { builtinVisualStyles } from "./visualStyles";

const builtinThemes: AnyPlugin[] = [
  flameDark,
  flameLight,
  atomOneDark,
  atomOneLight,
  tokyoNightStorm,
  tokyoNightLight,
  solarizedDark,
  solarizedLight,
  catppuccinMocha,
  catppuccinLatte,
];

export const appearancePlugins: AnyPlugin[] = [
  ...builtinThemes,
  customTheme,
  ...builtinVisualStyles,
  appearancePainter,
];
