import type { Scheme, VisualStyleMotion } from "@/lib/appearance";

export interface ColorThemeSpec {
  id: string;
  label: string;
  scheme: Scheme;
  icon?: string;
  order?: number;
  tokens?: Record<string, string>;
}

export interface AccentSpec {
  id: string;
  label: string;
  dark: string;
  light?: string;
  order?: number;
}

export interface VisualStyleSpec {
  id: string;
  motion: VisualStyleMotion;
  tokens: Record<string, string>;
}
