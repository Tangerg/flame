import { FLAME_ORANGE } from "@flame/brand-assets/color";
export type ColorThemeId = string;

export type VisualStyleId = string;

export const UI_DENSITY_MODES = ["compact", "comfortable", "spacious"] as const;

export type UiDensity = (typeof UI_DENSITY_MODES)[number];

export const DEFAULT_UI_DENSITY: UiDensity = "comfortable";

export const DEFAULT_CONTRAST = 25;

export const DEFAULT_ACCENT = FLAME_ORANGE;

export const DEFAULT_CUSTOM_THEME = { bg: "#0f1117", fg: "#e6e8ee" } as const;

export const PAPER_TEXTURES = ["off", "xuan", "cotton", "grain"] as const;

export type PaperTexture = (typeof PAPER_TEXTURES)[number];

const DEFAULT_PAPER_TEXTURE_STRENGTH = 3;

interface CustomTheme {
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
  codeFontSize: number | null;
  fontSmoothing: boolean;
  density: UiDensity;
  radiusScale: number;
  motionScale: number;
  paperTexture: PaperTexture;
  paperTextureStrength: number;
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
  setCodeFontSize: (size: number | null) => void;
  setFontSmoothing: (on: boolean) => void;
  setDensity: (density: UiDensity) => void;
  setRadiusScale: (scale: number) => void;
  setMotionScale: (scale: number) => void;
  setPaperTexture: (texture: PaperTexture) => void;
  setPaperTextureStrength: (strength: number) => void;
}

export function defaultAppearance(): AppearancePreference {
  return {
    theme: "system",
    visualStyle: "flame",
    accent: DEFAULT_ACCENT,
    customTheme: { ...DEFAULT_CUSTOM_THEME },
    contrast: DEFAULT_CONTRAST,
    uiFont: "",
    codeFont: "",
    fontSize: null,
    codeFontSize: null,
    fontSmoothing: true,
    density: DEFAULT_UI_DENSITY,
    radiusScale: 1,
    motionScale: 1,
    paperTexture: "off",
    paperTextureStrength: DEFAULT_PAPER_TEXTURE_STRENGTH,
  };
}
