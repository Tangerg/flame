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
   * undefined — Solarized's base3 is Solarized, not a tint of the selected accent.
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

export type RegionLayout = "floating-card" | "flush-panes" | "tonal-columns" | "tool-windows";
export type ControlTreatment = "quiet" | "outlined" | "tonal" | "elevated";

export interface VisualStylePreview {
  canvas: string;
  sidebar: string;
  dock: string;
  edge: string;
  accent: string;
}

/**
 * A complete component and region design language, independent from colour. `traits` expose
 * structural intent to shell CSS as data attributes while tokens own metrics and materials,
 * and keeping both in ONE contribution is what lets a third-party style change pane
 * relationships rather than merely repaint existing controls.
 */
export interface VisualStyleSpec {
  id: string;
  label: string;
  description: string;
  order?: number;
  traits: {
    regions: RegionLayout;
    controls: ControlTreatment;
  };
  motion: VisualStyleMotion;
  preview: VisualStylePreview;
  /** CSS custom properties without the leading `--`. */
  tokens: Record<string, string>;
}
