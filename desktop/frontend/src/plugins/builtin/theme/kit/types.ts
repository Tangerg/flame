// Its own file so `tokens.ts` can import these without forming a cycle with
// `defineColorThemePlugin.ts`, which imports token defaults back from `tokens.ts`.

import type { ThemeNeutralSteps } from "@/plugins/sdk";
import type { Scheme } from "@/lib/appearance";

export interface ThemeBrand {
  accent: string;
  textOnAccent: string;
  /** Defaults to `colord(accent).darken(0.08)`. */
  accentBorder?: string;
  /** Defaults to `colord(accent).darken(0.16)`. */
  accentPress?: string;
}

/**
 * The surface-2/-3/-4 steps are ALWAYS derived by color-mix off `--depth-step`; a theme
 * cannot pin them, or the contrast slider goes partially dead. `elevated` and `sunken` are
 * anchors rather than rungs on that ladder: it walks one direction only, toward the ink,
 * while a card lifts away from the ink on light and toward it on dark.
 */
export interface ThemeSurfaces {
  bg: string;
  surface: string;
  /** Defaults to the first ladder step. */
  elevated?: string;
  /** Defaults to a fixed per-scheme neutral, deliberately OFF the ladder: a control's own
   *  fill must not drift when the contrast slider moves. */
  sunken?: string;
}

/** See DESIGN.md §2 for the hierarchy each step carries. */
export interface ThemeInk {
  /** The anchor: the soft/muted/faint ramp derives from this when omitted. */
  text: string;
  textBright: string;
  /** Omit to auto-derive at ~82% alpha, which adapts to the surface behind it. Pin a hue
   *  only when the palette's own ink ramp is intentional (Solarized, Catppuccin). */
  textSoft?: string;
  /** Omit to auto-derive (~56% alpha). Must clear WCAG AA at 11-12px. */
  textMuted?: string;
  /** Omit to auto-derive (~38% alpha). Must clear WCAG AA at 11-12px on canvas AND surface. */
  textFaint?: string;
}

/** Literal hex, NOT alpha-blended (DESIGN.md §2), so borders read as precise. */
export interface ThemeBorders {
  border: string;
  borderSoft: string;
  divider: string;
}

/** Used SPARINGLY per DESIGN.md §9 — never decoratively. */
export interface ThemeSemantic {
  negative: string;
  warning: string;
  info: string;
  /** NOT the brand accent: accent means "live", success means "finished cleanly". */
  success: string;
}

/** Defaults to accent-driven, but a theme may override it so the accent stays reserved
 *  for "live" state. */
export interface ThemeCta {
  cta: string;
  ctaHover: string;
  ctaText: string;
}

export interface ColorThemePluginSpec {
  /** Persisted by `uiStore`, so renaming one strands a user's saved choice. */
  id: string;
  label: string;
  /** Drives the structural `theme-{scheme}` class and scheme-aware assets. */
  scheme: Scheme;
  icon?: string;
  /** Lower comes first. */
  order?: number;

  brand: ThemeBrand;
  surfaces: ThemeSurfaces;
  ink: ThemeInk;
  borders: ThemeBorders;
  semantic: ThemeSemantic;

  cta?: Partial<ThemeCta>;

  /**
   * Opt in to having the neutral family follow the LIVE accent; `surfaces` and `borders`
   * stay the first-paint values for the default accent. A palette theme must NOT set it —
   * Solarized's base3 is Solarized, not a tint of the selected accent.
   */
  neutralSteps?: ThemeNeutralSteps;

  /**
   * Escape hatch for palette variables not captured by the typed sections.
   * Geometry, elevation and motion belong to a visual-style contribution.
   * Keys are CSS-variable names WITHOUT the leading `--`.
   */
  extras?: Record<string, string>;
}
