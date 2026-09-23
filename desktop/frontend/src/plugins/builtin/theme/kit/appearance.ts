export type ColorThemeId = string;
export type VisualStyleId = string;

export const UI_DENSITY_MODES = ["compact", "comfortable", "spacious"] as const;
export type UiDensity = (typeof UI_DENSITY_MODES)[number];
export const DEFAULT_UI_DENSITY: UiDensity = "comfortable";

export const DEFAULT_CONTRAST = 25;

export interface CustomTheme {
  bg: string;
  fg: string;
}

export interface AppearancePreference {
  theme: ColorThemeId;
  visualStyle: VisualStyleId;
  accent: string;
  customTheme: CustomTheme;
  contrast: number;
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
  setUiFont: (font: string) => void;
  setCodeFont: (font: string) => void;
  setFontSize: (size: number | null) => void;
  setFontSmoothing: (on: boolean) => void;
  setDensity: (density: UiDensity) => void;
  setRadiusScale: (scale: number) => void;
  setMotionScale: (scale: number) => void;
}
