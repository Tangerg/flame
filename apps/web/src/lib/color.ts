import {
  modeRgb,
  modeHsl,
  modeLrgb,
  modeOklab,
  modeOklch,
  useMode as registerColorMode,
} from "culori/fn";

// Culori's function entry only enables the spaces used by the product.
export const rgbColor = registerColorMode(modeRgb);
export const oklchColor = registerColorMode(modeOklch);
registerColorMode(modeHsl);
registerColorMode(modeLrgb);
registerColorMode(modeOklab);

export {
  blend,
  clampChroma,
  formatHex,
  formatHex8,
  interpolate,
  wcagContrast,
  wcagLuminance,
} from "culori/fn";
