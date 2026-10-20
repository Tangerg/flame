import { type Scheme } from "@/lib/appearance";
import { type ColorThemeSpec } from "@/features/theme/public/types";
import { edgeOnCanvas, focusOnCanvas } from "@/features/theme/kit/legibility";
import { mixOklab, toOklch } from "@/features/theme/kit/oklab";
import { hueScale, neutralScale, solidInk, step, type Scale } from "@/features/theme/kit/scale";
import { SCHEME_ICON, inkTracksLadder } from "@/features/theme/kit/tokens";
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
  dark: { background: "#0f0f0f", foreground: "#eeeeee" },
  light: { background: "#f5f5f5", foreground: "#1b1b1b" },
};

const STATUS_SEED = {
  negative: "#e5484d",
  warning: "#ffc53d",
  success: "#30a46c",
  info: "#0090ff",
} as const;

const PLANE_STEP: Record<
  Scheme,
  Record<"drawer" | "surface" | "sunken" | "divider" | "border" | "borderStrong", number>
> = {
  light: { drawer: 2, surface: 2, sunken: 3, divider: 4, border: 5, borderStrong: 7 },
  dark: { drawer: 1, surface: 3, sunken: 4, divider: 4, border: 5, borderStrong: 7 },
};

function elevatedPlane(scheme: Scheme, neutral: Scale): string {
  return scheme === "light"
    ? mixOklab("#ffffff", step(neutral, 1), 85)
    : step(neutral, PLANE_STEP.dark.surface);
}

export function paletteThemeContribution(
  theme: PaletteTheme,
  preferredAccent: string,
  contrast: number,
): ColorThemeSpec {
  const { scheme, palette } = theme;
  const background = palette.background ?? SCHEME_BASE[scheme].background;
  const foreground = palette.foreground ?? SCHEME_BASE[scheme].foreground;
  const seed = palette.accent ?? preferredAccent;
  const separation = 0.6 + 1.6 * (Math.min(100, Math.max(0, contrast)) / 100);
  const planes = PLANE_STEP[scheme];
  const neutral = neutralScale(
    background,
    foreground,
    scheme,
    palette.background === undefined ? toOklch(seed).h : null,
    separation,
    planes.sunken,
  );
  const canvas = step(neutral, 1);
  const textPlanes = [step(neutral, planes.sunken), elevatedPlane(scheme, neutral)];
  const scale = (color: string) => hueScale(color, scheme, canvas, textPlanes);
  const accent = scale(seed);
  const status = {
    negative: scale(STATUS_SEED.negative),
    warning: scale(STATUS_SEED.warning),
    success: scale(STATUS_SEED.success),
    info: scale(STATUS_SEED.info),
  };
  const solid = step(accent, 9);
  const onSolid = solidInk(solid);
  const text = step(neutral, 12);

  return {
    id: theme.id,
    label: theme.label,
    scheme,
    icon: theme.icon ?? SCHEME_ICON[scheme],
    order: theme.order,
    tokens: {
      "color-accent": solid,
      "color-text-on-accent": onSolid,
      "color-accent-line": step(accent, 7),
      "color-accent-text": step(accent, 11),
      "color-accent-strong": step(accent, 12),
      "tone-accent": solid,

      "color-bg": canvas,
      "color-drawer": step(neutral, planes.drawer),
      "color-surface": step(neutral, planes.surface),
      "color-elevated": elevatedPlane(scheme, neutral),
      "color-sunken": step(neutral, planes.sunken),

      "color-text": text,
      "color-text-bright": mixOklab(text, scheme === "dark" ? "#ffffff" : "#000000", 80),
      "color-text-soft": inkTracksLadder(scheme, mixOklab(text, step(neutral, 11), 55)),
      "color-text-muted": inkTracksLadder(scheme, palette.muted ?? step(neutral, 11)),
      "color-text-faint": inkTracksLadder(scheme, step(neutral, 10)),

      "color-focus-ring": focusOnCanvas(solid, text, canvas),
      "color-control-edge": edgeOnCanvas(text, canvas),
      "color-divider": step(neutral, planes.divider),
      "color-border": palette.border ?? step(neutral, planes.border),
      "color-border-strong": step(neutral, planes.borderStrong),

      "color-negative": step(status.negative, 11),
      "color-warning": step(status.warning, 11),
      "color-success": step(status.success, 11),
      "color-info": step(status.info, 11),
      "tone-negative": step(status.negative, 9),
      "tone-warning": step(status.warning, 9),
      "tone-success": step(status.success, 9),
      "tone-info": step(status.info, 9),

      "color-cta": solid,
      "color-cta-hover": step(accent, 10),
      "color-cta-text": onSolid,
    },
  };
}
