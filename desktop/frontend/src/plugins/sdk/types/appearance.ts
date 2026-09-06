import type { Scheme, VisualStyleMotion } from "@/lib/appearance";

/** A swappable colour palette. Geometry and component treatment belong to a visual style. */
export interface ColorThemeSpec {
  /** Stable id persisted by the UI preference store. */
  id: string;
  label: string;
  scheme: Scheme;
  icon?: string;
  order?: number;
  /** CSS custom properties without the leading `--`. */
  tokens?: Record<string, string>;
  /**
   * Opt in to having the neutral family follow the LIVE accent; `tokens` stays the family at
   * the default accent and is what a cold boot paints. A palette theme MUST leave this
   * undefined — a palette theme's own surface is its own, not a tint of the selected accent.
   */
  neutralSteps?: ThemeNeutralSteps;
}

/** One neutral's place: OKLCH lightness (0-100) and chroma at the reference accent. */
export interface NeutralStep {
  l: number;
  c: number;
}

export interface ThemeNeutralSteps {
  surface: NeutralStep;
  elevated: NeutralStep;
  sunken: NeutralStep;
  border: NeutralStep;
  borderSoft: NeutralStep;
}

export interface AccentSpec {
  id: string;
  label: string;
  dark: string;
  light?: string;
  order?: number;
}

/**
 * A complete component and region design language, independent from colour. Tokens are the
 * whole of it: metrics, materials and region relationships are all custom properties, so a
 * third-party style changes how panes relate rather than merely repainting controls.
 */
export interface VisualStyleSpec {
  id: string;
  motion: VisualStyleMotion;
  /** CSS custom properties without the leading `--`. */
  tokens: Record<string, string>;
}
