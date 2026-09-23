import { colord } from "colord";
import type { Scheme } from "@/lib/appearance";
import { DEFAULT_CONTRAST } from "./appearance";
import type { ColorThemePluginSpec, ThemeCta } from "./types";

export const SCHEME_ICON: Record<Scheme, string> = {
  dark: "moon",
  light: "sun",
};

const SCHEME_SUNKEN: Record<Scheme, string> = {
  dark: "#1c1c21",
  light: "#f1f1f4",
};

export function depthStep(scheme: Scheme, contrast: number): string {
  const step = (2 + (contrast / 100) * 8) * (scheme === "dark" ? 2 : 1);
  return `${Number(step.toFixed(1))}%`;
}

const INK_TRACKING = 3;

export function buildTokenMap(spec: ColorThemePluginSpec): Record<string, string> {
  const inkAlpha = (pct: number) => `color-mix(in oklab, var(--color-text) ${pct}%, transparent)`;

  const rest = depthStep(spec.scheme, DEFAULT_CONTRAST);
  const tracksLadder = (ink: string) =>
    `color-mix(in oklab, var(--color-text) max(0%, calc((var(--depth-step) - ${rest}) * ${INK_TRACKING})), ${ink})`;

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

    "color-bg": spec.surfaces.bg,
    "color-surface": spec.surfaces.surface,
    "color-elevated": spec.surfaces.elevated ?? "var(--color-surface-2)",
    "color-sunken": spec.surfaces.sunken ?? SCHEME_SUNKEN[spec.scheme],
    "color-drawer":
      spec.surfaces.drawer ??
      (spec.scheme === "dark"
        ? colord(spec.surfaces.bg).darken(0.035).toHex()
        : spec.surfaces.surface),

    "color-text": spec.ink.text,
    "color-text-bright": spec.ink.textBright,
    "color-text-soft": tracksLadder(spec.ink.textSoft ?? inkAlpha(82)),
    "color-text-muted": tracksLadder(spec.ink.textMuted ?? inkAlpha(56)),
    "color-text-faint": tracksLadder(spec.ink.textFaint ?? spec.ink.textMuted ?? inkAlpha(56)),

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
