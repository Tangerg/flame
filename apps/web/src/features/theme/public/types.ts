import { type Scheme } from "@/lib/appearance";
import { type VisualStyleMotion } from "@/lib/visualStyleMotion";
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
  color: string;
  order?: number;
}

export interface VisualStyleSpec {
  id: string;
  motion: VisualStyleMotion;
  tokens: Record<string, string>;
}
