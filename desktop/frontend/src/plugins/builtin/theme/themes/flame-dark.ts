// The INVERSION of the light scheme: the reading plane is the DARKEST surface and the
// chrome steps up around it. Cards sit at the chrome's value, so an object on the plane
// reads as lifted while the same object inside a tool window reads as flush. One hue,
// chroma by area — same policy and reason as the light scheme.

import { defineColorThemePlugin } from "../kit/defineColorThemePlugin";

const c = {
  // Its hue is what every neutral below is tuned to.
  accent: "#3574f0",

  canvas: "#1d1f23",
  surface1: "#2a2d32",
  sunken: "#14181f",

  // surface2/3/4 derive as ink mixes off `surface1`: those are CHIP rungs, not region
  // materials.

  inkBright: "#ffffff",
  ink: "#e3e5e9",
  inkSoft: "#c6c9cf",
  inkMuted: "#aaaeb5",
  inkFaint: "#95999f",

  // A CONTROL's edge is a real line; a REGION's edge is not — region separation is the
  // visual style's cast, so nothing in this ramp is ever stretched into a pane divider.
  // Percentages of white over the ground (4 / 8 / 16), not picked greys. Dark carries a
  // wider top step than light because a light line on a dark ground loses more of itself
  // to the surround.
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
    // The SAME material as the chrome: on the darker plane it reads as lifted without a
    // second value.
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
  // Lifted far enough to clear AA on EVERY ground each one lands on — for negative that
  // means the card and its own wash, not just the canvas.
  semantic: {
    negative: "#e68a8a",
    warning: "#d6a750",
    // The accent's hue lifted 12 L, NOT `c.accent`: a fill is read as an AREA and an ink
    // as LETTERS. Aliased to the brand fill, a 12px status label measures 3.23:1 on a card.
    info: "#6e9bf4",
    success: "#6db473",
  },
  // Where each neutral sits, so the shell can rewrite them onto the live accent. The
  // hexes above are GENERATED from these steps at this theme's own accent — they are
  // what a cold boot paints, and the derivation reproduces them byte for byte while the
  // accent is untouched. Edit a step, regenerate the literal; they are one fact.
  neutralSteps: {
    surface: { l: 29.6, c: 0.01 },
    elevated: { l: 29.6, c: 0.01 },
    sunken: { l: 20.9, c: 0.015 },
    border: { l: 31.2, c: 0.0095 },
    borderSoft: { l: 37.9, c: 0.0107 },
  },
  // The primary CTA IS the accent here, not an inverting ink button: this
  // language spends colour on the one action that matters and keeps everything
  // else grey. Following `--color-accent` rather than a literal keeps the user's
  // accent pick on the button too.
  // One shade below the indicator accent, because the dark-scheme accents are
  // tuned to glow against near-black and carry white label text at only
  // 3.3–4.6:1. The light scheme needs no such step — see flame-light.
  cta: {
    cta: "var(--color-accent-border)",
    ctaHover: "var(--color-accent-press)",
    ctaText: "var(--color-text-on-accent)",
  },
});
