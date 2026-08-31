// The theme context's own vocabulary. `Scheme` and `VisualStyleMotion` are NOT here: those
// are published down to rings that cannot import a plugin (`lib/motion` reads the motion,
// `lib/highlight` reads the scheme), so they belong to the publication seam in
// `lib/appearance`. Everything below is read only by this context and the pane that edits it.

export type ColorThemeId = string;
export type VisualStyleId = string;

export const ACCENT_TINTS = ["off", "soft", "standard"] as const;
export type AccentTint = (typeof ACCENT_TINTS)[number];
export const DEFAULT_ACCENT_TINT: AccentTint = "standard";

export const UI_DENSITY_MODES = ["compact", "comfortable", "spacious"] as const;
export type UiDensity = (typeof UI_DENSITY_MODES)[number];
export const DEFAULT_UI_DENSITY: UiDensity = "comfortable";

export interface CustomTheme {
  bg: string;
  fg: string;
}

/** What the painter reads and the pane edits. Declared apart from the store so
 *  `installDocumentAppearance` accepts any carrier of these values — which is how the
 *  visual harness drives the real pipeline off a fixture. */
export interface AppearancePreference {
  theme: ColorThemeId;
  visualStyle: VisualStyleId;
  accent: string;
  customTheme: CustomTheme;
  contrast: number;
  accentTint: AccentTint;
  uiFont: string;
  codeFont: string;
  fontSize: number | null;
  fontSmoothing: boolean;
  density: UiDensity;
  radiusScale: number;
  motionScale: number;
}

export interface AppearanceEdit {
  setTheme: (theme: ColorThemeId) => void;
  setVisualStyle: (visualStyle: VisualStyleId) => void;
  setAccent: (accent: string) => void;
  setCustomTheme: (patch: Partial<CustomTheme>) => void;
  setContrast: (contrast: number) => void;
  setAccentTint: (accentTint: AccentTint) => void;
  setUiFont: (font: string) => void;
  setCodeFont: (font: string) => void;
  setFontSize: (size: number | null) => void;
  setFontSmoothing: (on: boolean) => void;
  setDensity: (density: UiDensity) => void;
  setRadiusScale: (scale: number) => void;
  setMotionScale: (scale: number) => void;
}
