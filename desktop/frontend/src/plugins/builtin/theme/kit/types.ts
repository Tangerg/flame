import type { Scheme } from "@/lib/appearance";

interface ThemeBrand {
  accent: string;
  textOnAccent: string;
  accentBorder?: string;
  accentPress?: string;
}

interface ThemeSurfaces {
  bg: string;
  surface: string;
  elevated?: string;
  sunken?: string;
  drawer?: string;
}

interface ThemeInk {
  text: string;
  textBright: string;
  textSoft?: string;
  textMuted?: string;
  textFaint?: string;
}

interface ThemeBorders {
  border: string;
  borderSoft: string;
  divider: string;
}

interface ThemeSemantic {
  negative: string;
  warning: string;
  info: string;
  success: string;
}

export interface ThemeCta {
  cta: string;
  ctaHover: string;
  ctaText: string;
}

export interface ColorThemePluginSpec {
  id: string;
  label: string;
  scheme: Scheme;
  icon?: string;
  order?: number;

  brand: ThemeBrand;
  surfaces: ThemeSurfaces;
  ink: ThemeInk;
  borders: ThemeBorders;
  semantic: ThemeSemantic;

  cta?: Partial<ThemeCta>;

  extras?: Record<string, string>;
}
