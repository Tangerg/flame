import { colord } from "colord";
import type { Scheme } from "@/lib/appearance";
import type { ColorThemeSpec } from "@/plugins/sdk";
import { colorThemeContribution } from "./colorThemeContribution";
import { WCAG_AA_TEXT, legibleMix, mixOklab } from "./legibility";

interface ThemePalette {
  background?: string;
  foreground?: string;
  accent?: string;
  muted?: string;
  border?: string;
}

export interface PaletteTheme {
  id: string;
  label: string;
  scheme: Scheme;
  icon?: string;
  order?: number;
  palette: ThemePalette;
}

export const SCHEME_BASE: Record<Scheme, { background: string; foreground: string }> = {
  dark: { background: "#1f1f1f", foreground: "#e3e5e9" },
  light: { background: "#ffffff", foreground: "#1e1f22" },
};

const mix = (a: string, pct: number, b: string): string =>
  `color-mix(in oklab, ${a} ${pct}%, ${b})`;

export function paletteThemeContribution(
  theme: PaletteTheme,
  preferredAccent: string,
  contrast: number,
): ColorThemeSpec {
  const { scheme, palette } = theme;
  const bg = palette.background ?? SCHEME_BASE[scheme].background;
  const fg = palette.foreground ?? SCHEME_BASE[scheme].foreground;
  const accent = palette.accent ?? preferredAccent;
  const k = Math.min(100, Math.max(0, contrast)) / 100;
  const p = (lo: number, hi: number) => Math.round(lo + (hi - lo) * k);
  const extreme = scheme === "dark" ? "#ffffff" : "#000000";
  const chromePct = p(4, 12);
  const chrome = mix(fg, chromePct, bg);
  const plane = mixOklab(fg, bg, chromePct);
  const legible = (floorPct: number) => legibleMix(fg, bg, plane, floorPct, WCAG_AA_TEXT);
  return {
    ...colorThemeContribution({
      id: theme.id,
      label: theme.label,
      scheme,
      icon: theme.icon,
      order: theme.order,
      brand: { accent, textOnAccent: colord(accent).isDark() ? "#ffffff" : "#000000" },
      surfaces: {
        bg,
        surface: chrome,
        elevated: scheme === "dark" ? chrome : mix("#ffffff", p(35, 80), bg),
        sunken: mix("#000000", p(3, 8), bg),
      },
      ink: {
        text: fg,
        textBright: mix(fg, 80, extreme),
        textSoft: mix(fg, legible(p(86, 94)), bg),
        textMuted: palette.muted ?? mix(fg, legible(p(45, 75)), bg),
        textFaint: mix(fg, legible(p(28, 52)), bg),
      },
      borders: {
        border: palette.border ?? mix(fg, p(8, 22), bg),
        borderSoft: mix(fg, p(14, 32), bg),
        divider: mix(fg, p(5, 13), bg),
      },
      semantic: { negative: "#e5484d", warning: "#f5a623", info: "#3b82f6", success: "#30a46c" },
    }),
    accent: palette.accent,
  };
}
