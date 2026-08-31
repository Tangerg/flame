import { defineColorThemePlugin } from "../kit/defineColorThemePlugin";

const c = {
  accent: "#3574f0",

  canvas: "#1d1f23",
  surface1: "#2a2d32",
  sunken: "#14181f",

  inkBright: "#ffffff",
  ink: "#e3e5e9",
  inkSoft: "#c6c9cf",
  inkMuted: "#aaaeb5",
  inkFaint: "#95999f",

  hairline: "#2e3136",
  hairStrong: "#3f4248",
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
    // Lifted 12 L, NOT `c.accent`: aliased to the brand fill a 12px label reads 3.23:1.
    info: "#6e9bf4",
    success: "#6db473",
  },
  // The hexes above are GENERATED from these steps at this theme's own accent. Edit a step,
  // regenerate the literal — they are one fact.
  neutralSteps: {
    surface: { l: 29.6, c: 0.01 },
    elevated: { l: 29.6, c: 0.01 },
    sunken: { l: 20.9, c: 0.015 },
    border: { l: 31.2, c: 0.0095 },
    borderSoft: { l: 37.9, c: 0.0107 },
  },
  cta: {
    cta: "var(--color-accent-border)",
    ctaHover: "var(--color-accent-press)",
    ctaText: "var(--color-text-on-accent)",
  },
});
