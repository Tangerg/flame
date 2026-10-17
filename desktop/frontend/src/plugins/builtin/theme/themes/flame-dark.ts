import { defineColorThemePlugin } from "../kit/defineColorThemePlugin";
import { SCHEME_BASE } from "../kit/palette";

const c = {
  accent: "#0485f7",

  canvas: SCHEME_BASE.dark.background,
  surface1: "#18181b",
  sunken: "#232325",

  inkBright: "#ffffff",
  ink: SCHEME_BASE.dark.foreground,
  inkSoft: "#d4d4d8",
  inkMuted: "#9f9fa9",
  inkFaint: "#9f9fa9",

  hairline: "#28282c",
  hairStrong: "#3f3f46",
  hairTertiary: "#212124",
};

export default defineColorThemePlugin({
  id: "dark",
  label: "Dark",
  scheme: "dark",
  order: 0,

  brand: {
    accent: c.accent,
    textOnAccent: "#ffffff",
  },
  surfaces: {
    bg: c.canvas,
    drawer: c.canvas,
    surface: c.surface1,
    elevated: c.surface1,
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
    negative: "#e68a8a",
    warning: "#d6a750",
    info: "#6e9bf4",
    success: "#6db473",
  },
  cta: {
    cta: "#006fdb",
    ctaHover: "#0064c5",
    ctaText: "#ffffff",
  },
});
