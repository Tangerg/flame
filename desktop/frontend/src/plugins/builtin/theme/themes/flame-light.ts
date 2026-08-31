import { defineColorThemePlugin } from "../kit/defineColorThemePlugin";

const c = {
  accent: "#2b5fd0",

  canvas: "#ffffff",
  card: "#f7faff",
  surface1: "#f4f6fa",
  sunken: "#eaf0fb",

  inkBright: "#000000",
  ink: "#1e1f22",
  inkSoft: "#3d4147",
  inkMuted: "#5a5d63",
  inkFaint: "#63666d",

  hairline: "#e9ecf2",
  hairStrong: "#dee2eb",
  hairTertiary: "rgb(0 0 0 / 0.05)",
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
  // The hexes above are the same family at the default accent. Edit a step, regenerate the
  // literal — they are one fact.
  neutralSteps: {
    surface: { l: 97.3, c: 0.006 },
    elevated: { l: 98.4, c: 0.008 },
    sunken: { l: 95.4, c: 0.016 },
    border: { l: 94.3, c: 0.009 },
    borderSoft: { l: 91.2, c: 0.013 },
  },
  cta: {
    cta: "var(--color-accent)",
    ctaHover: "var(--color-accent-border)",
    ctaText: "var(--color-text-on-accent)",
  },
});
