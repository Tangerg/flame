import { type Scheme } from "@/lib/appearance";
import { type Label, type MessageKey } from "@/lib/i18n/index";
import { type VisualStyleMotion } from "@/lib/visualStyleMotion";
export interface ColorThemeSpec {
  id: string;
  label: Label;
  scheme: Scheme;
  icon?: string;
  order?: number;
  tokens?: Record<string, string>;
}

export interface AccentSpec {
  id: string;
  label: MessageKey;
  color: string;
  order?: number;
}

export interface VisualStyleSpec {
  id: string;
  motion: VisualStyleMotion;
  tokens: Record<string, string>;
}
