import { defineColorThemePlugin } from "../kit/defineColorThemePlugin";

const c = {
  accent: "#3574f0",

  canvas: "#1f1f1f",
  surface1: "#2b2b2b",
  sunken: "#181818",

  inkBright: "#ffffff",
  ink: "#e3e5e9",
  inkSoft: "#c6c9cf",
  inkMuted: "#aaaeb5",
  inkFaint: "#9da1a7",

  hairline: "#303030",
  hairStrong: "#414141",
  hairTertiary: "rgb(255 255 255 / 0.04)",
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
    cta: "var(--color-accent-border)",
    ctaHover: "var(--color-accent-press)",
    ctaText: "var(--color-text-on-accent)",
  },
});
