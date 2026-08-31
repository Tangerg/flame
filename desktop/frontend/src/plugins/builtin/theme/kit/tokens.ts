import { colord } from "colord";
import type { Scheme } from "@/lib/appearance";
import type { ColorThemePluginSpec, ThemeCta } from "./types";

export const SCHEME_ICON: Record<Scheme, string> = {
  dark: "moon",
  light: "sun",
};

/** A fixed neutral, deliberately OFF the ladder: a control's fill must hold still while the
 *  contrast slider moves the regions. */
const SCHEME_SUNKEN: Record<Scheme, string> = {
  dark: "#1c1c21",
  light: "#f1f1f4",
};

/** Doubled on dark: equal ink percentages do not buy equal separation, so one setting tuned
 *  on light flattens every dark scheme. `globals.css` mirrors both defaults. */
export function depthStep(scheme: Scheme, contrast: number): string {
  const step = (2 + (contrast / 100) * 8) * (scheme === "dark" ? 2 : 1);
  return `${step.toFixed(1)}%`;
}

/**
 * PURE. `accentBorder` / `accentPress` auto-derive from the accent unless overridden, CTA
 * defaults to accent-driven, and `extras` wins on collision.
 */
export function buildTokenMap(spec: ColorThemePluginSpec): Record<string, string> {
  // Mixed over transparent so it composites against whatever surface it sits on.
  const inkAlpha = (pct: number) => `color-mix(in oklab, var(--color-text) ${pct}%, transparent)`;

  const accent = colord(spec.brand.accent);
  const accentBorder = spec.brand.accentBorder ?? accent.darken(0.08).toHex();
  const accentPress = spec.brand.accentPress ?? accent.darken(0.16).toHex();
  const cta: ThemeCta = {
    cta: spec.brand.accent,
    ctaHover: accentBorder,
    ctaText: spec.brand.textOnAccent,
    ...spec.cta,
  };

  return {
    "color-accent": spec.brand.accent,
    "color-accent-border": accentBorder,
    "color-accent-press": accentPress,
    "color-text-on-accent": spec.brand.textOnAccent,

    // The -2/-3/-4 steps are the color-mix ladder in globals.css, never emitted here: they
    // track --depth-step. `elevated` / `sunken` are anchors because that ladder walks one way.
    "color-bg": spec.surfaces.bg,
    "color-surface": spec.surfaces.surface,
    "color-elevated": spec.surfaces.elevated ?? "var(--color-surface-2)",
    "color-sunken": spec.surfaces.sunken ?? SCHEME_SUNKEN[spec.scheme],

    // Faint shares the muted fallback on purpose: a third lower-opacity rung fails AA on
    // ordinary canvases.
    "color-text": spec.ink.text,
    "color-text-bright": spec.ink.textBright,
    "color-text-soft": spec.ink.textSoft ?? inkAlpha(82),
    "color-text-muted": spec.ink.textMuted ?? inkAlpha(56),
    "color-text-faint": spec.ink.textFaint ?? spec.ink.textMuted ?? inkAlpha(56),

    "color-border": spec.borders.border,
    "color-border-soft": spec.borders.borderSoft,
    "color-divider": spec.borders.divider,

    "color-negative": spec.semantic.negative,
    "color-warning": spec.semantic.warning,
    "color-info": spec.semantic.info,
    "color-success": spec.semantic.success,

    "color-cta": cta.cta,
    "color-cta-hover": cta.ctaHover,
    "color-cta-text": cta.ctaText,

    ...spec.extras,
  };
}
