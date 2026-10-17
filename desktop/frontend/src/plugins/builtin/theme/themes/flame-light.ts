import { defineColorThemePlugin } from "../kit/defineColorThemePlugin";
import { SCHEME_BASE } from "../kit/palette";

const c = {
  accent: "#006fdb",

  canvas: SCHEME_BASE.light.background,
  card: "#ffffff",
  surface1: "#efeff0",
  sunken: "#eaeaeb",

  inkBright: "#000000",
  ink: SCHEME_BASE.light.foreground,
  inkSoft: "#3f3f46",
  inkMuted: "#66666f",
  inkFaint: "#66666f",

  hairline: "#dedee0",
  hairStrong: "#c4c4c9",
  hairTertiary: "#e4e4e7",
};

export default defineColorThemePlugin({
  id: "light",
  label: "Light",
  scheme: "light",
  order: 0,

  brand: {
    accent: c.accent,
    textOnAccent: "#ffffff",
  },
  surfaces: {
    bg: c.canvas,
    surface: c.surface1,
    elevated: c.card,
    sunken: c.sunken,
  },
  ink: {
    text: c.ink,
    textBright: c.inkBright,
    textSoft: c.inkSoft,
    textMuted: c.inkMuted,
    textFaint: c.inkFaint,
  },
  borders: {
    border: c.hairline,
    borderSoft: c.hairStrong,
    divider: c.hairTertiary,
  },
  semantic: {
    negative: "#b0342b",
    warning: "#84610e",
    info: c.accent,
    success: "#2a713e",
  },
  cta: {
    cta: c.accent,
    ctaHover: "#0064c5",
    ctaText: "#ffffff",
  },
});
