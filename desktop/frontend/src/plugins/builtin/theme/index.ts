// Adding a theme is a file under `themes/` plus an entry in the array below: the manifest
// pulls in this one pack and never names an individual theme.

import type { AnyPlugin } from "dougong";
import { appearancePainter } from "./appearancePainter";
import customTheme from "./themes/custom-theme";
import flameDark from "./themes/flame-dark";
import flameLight from "./themes/flame-light";
import { builtinVisualStyles } from "./visualStyles";

// Two presets, and the product's own. Eight others shipped — ports of other editors'
// palettes — each carrying a full ladder that only flame-light and flame-dark were ever
// polished or visually regressed against. `custom` stays: it is not a preset but the seam
// that lets a reader bring their own three colours and derive the rest.
const builtinThemes: AnyPlugin[] = [flameDark, flameLight];

export const appearancePlugins: AnyPlugin[] = [
  ...builtinThemes,
  customTheme,
  ...builtinVisualStyles,
  appearancePainter,
];
