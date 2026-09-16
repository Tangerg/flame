import { defineColorThemePlugin } from "../kit/defineColorThemePlugin";

const c = {
  accent: "#2b5fd0",

  // Achromatic on purpose. These carried a blue cast — they were authored as "the neutral
  // family at the default blue accent", which is what the accent-tint derivation wanted. A
  // surface is not a shade of the accent; Codex draws its whole grey ramp with the channels
  // equal, and beside it ours read as a blue-grey panel where a near-white one belongs.
  canvas: "#ffffff",
  card: "#ffffff",
  surface1: "#f9f9f9",
  sunken: "#ededed",

  inkBright: "#000000",
  ink: "#1e1f22",
  inkSoft: "#3d4147",
  inkMuted: "#5a5d63",
  inkFaint: "#63666d",

  hairline: "#ededed",
  hairStrong: "#dfdfdf",
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
  cta: {
    cta: "var(--color-accent)",
    ctaHover: "var(--color-accent-border)",
    ctaText: "var(--color-text-on-accent)",
  },
});
