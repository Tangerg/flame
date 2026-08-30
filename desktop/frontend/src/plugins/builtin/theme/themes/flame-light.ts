// The reading plane is the brightest surface; everything else steps down from it, so an
// object on it steps IN rather than up.
//
// Every neutral sits on the accent's hue with chroma in INVERSE proportion to the area it
// covers. Below roughly C 0.005 a grey's hue is not addressable in 8-bit sRGB — one byte
// swings it 20-40° — so a lower ramp drifts across quantisation noise, which is what "dirty
// grey" means. Semantic hues are pulled one step deeper: the reference greens and ambers
// land at 3.4-3.9:1 as text on white.

import { defineColorThemePlugin } from "../kit/defineColorThemePlugin";

const c = {
  // Deeper than the dark scheme's blue: that one reads at 3.8:1 on this chrome. Same hue,
  // AA-clean — and it is the hue every neutral above is tuned to.
  accent: "#2b5fd0",

  canvas: "#ffffff",
  card: "#f7faff",
  // 2.7 L under the plane, not 4.2: at the deeper step the column reads as grey rather than
  // as paper of a different weight. A lighter panel, and a hairline that carries the seam.
  surface1: "#f4f6fa",
  sunken: "#eaf0fb",

  inkBright: "#000000",
  ink: "#1e1f22",
  inkSoft: "#3d4147",
  inkMuted: "#5a5d63",
  inkFaint: "#63666d",

  // Ink PERCENTAGES over the plane (5 / 8 / 12), not picked greys. Heavier seams make a
  // window of tool panels read as a wireframe of boxes rather than paper of different
  // weights; the value delta between panels was never the problem.
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
  // Where each neutral SITS, so the shell can rewrite them onto the live accent. The hexes
  // above are the same family at the default accent — what a cold boot paints before this
  // runs. See kit/accentTint for the rule.
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
