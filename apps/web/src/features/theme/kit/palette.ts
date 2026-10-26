import { type Scheme } from "@/lib/appearance";
import { type ColorThemeSpec } from "@/features/theme/public/types";
import { type AppearancePreference } from "@/features/theme/kit/appearance";
import {
  WCAG_AA_TEXT,
  compositeColor,
  controlEdge,
  focusRing,
  legibleMix,
  relativeLuminance,
  type ContrastColor,
} from "@/features/theme/kit/legibility";
import { mixOklab } from "@/features/theme/kit/oklab";
import { paperTextureBackgrounds } from "@/features/theme/kit/paperTexture";
import {
  hueScale,
  neutralScale,
  solidFill,
  solidInk,
  step,
  type Scale,
} from "@/features/theme/kit/scale";
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

const WASH_OPACITY = 0.1;
const SELECTION_MIX = { selected: 10, selectedHover: 15 } as const;
const wash = (tone: "accent" | keyof typeof STATUS_SEED) =>
  `color-mix(in srgb, var(--tone-${tone}) ${WASH_OPACITY * 100}%, transparent)`;
const selectionPaint = (percent: number) =>
  `color-mix(in srgb, var(--color-accent) ${percent}%, var(--color-bg))`;

function selectionStrength(
  accent: string,
  canvas: string,
  planes: readonly ContrastColor[],
): number {
  const luminances = planes.map(relativeLuminance);
  const min = Math.min(...luminances);
  const max = Math.max(...luminances);
  // Opaque state fills stay inside the qualified reading planes' luminance interval.
  for (let strength = 100; strength > 0; strength -= 1) {
    if (
      Object.values(SELECTION_MIX).every((mix) => {
        const percent = (mix * strength) / 100;
        const luminance = relativeLuminance(compositeColor(accent, canvas, percent / 100));
        return luminance >= min && luminance <= max;
      })
    )
      return strength;
  }
  return 0;
}

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
  paper?: Pick<AppearancePreference, "paperTexture" | "paperTextureStrength">,
): ColorThemeSpec {
  const { scheme, palette } = theme;
  const background = palette.background ?? SCHEME_BASE[scheme].background;
  const foreground = palette.foreground ?? SCHEME_BASE[scheme].foreground;
  const seed = palette.accent ?? preferredAccent;
  const separation = 0.6 + 1.6 * (Math.min(100, Math.max(0, contrast)) / 100);
  const planes = PLANE_STEP[scheme];
  const neutral = neutralScale(background, foreground, scheme, separation, planes.sunken);
  const canvas = step(neutral, 1);
  const drawer = step(neutral, planes.drawer);
  const surface = step(neutral, planes.surface);
  const elevated = elevatedPlane(scheme, neutral);
  const sunken = step(neutral, planes.sunken);
  // Only the canvas, drawer and surface carry paper; their worst-case painted planes join the
  // legibility checks so every ink derived below stays readable over the texture.
  const paperPlanes = paper
    ? paperTextureBackgrounds(paper.paperTexture, paper.paperTextureStrength, scheme, [
        canvas,
        drawer,
        surface,
      ])
    : [];
  const textPlanes = [sunken, elevated, ...paperPlanes];
  const backgrounds = [canvas, drawer, surface, elevated, sunken, ...paperPlanes];
  const scale = (color: string) => {
    const solid = solidFill(color);
    const washedPlanes = backgrounds.map((plane) => compositeColor(solid, plane, WASH_OPACITY));
    return hueScale(solid, scheme, canvas, [...textPlanes, ...washedPlanes]);
  };
  const accent = scale(seed);
  const status = {
    negative: scale(STATUS_SEED.negative),
    warning: scale(STATUS_SEED.warning),
    success: scale(STATUS_SEED.success),
    info: scale(STATUS_SEED.info),
  };
  const solid = step(accent, 9);
  const selection = selectionStrength(solid, canvas, backgrounds);
  const onSolid = solidInk(solid);
  const text = step(neutral, 12);
  const restingInk = (declared: string) =>
    inkTracksLadder(
      scheme,
      paperPlanes.length > 0
        ? mixOklab(text, declared, legibleMix(text, declared, backgrounds, 0, WCAG_AA_TEXT))
        : declared,
    );

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
      "color-accent-wash": wash("accent"),

      "color-bg": canvas,
      "color-drawer": drawer,
      "color-surface": surface,
      "color-elevated": elevated,
      "color-sunken": sunken,
      "wash-hover": sunken,
      "wash-selected": selectionPaint((SELECTION_MIX.selected * selection) / 100),
      "wash-selected-hover": selectionPaint((SELECTION_MIX.selectedHover * selection) / 100),

      "color-text": text,
      "color-text-bright": mixOklab(text, scheme === "dark" ? "#ffffff" : "#000000", 80),
      "color-text-soft": restingInk(mixOklab(text, step(neutral, 11), 55)),
      "color-text-muted": restingInk(palette.muted ?? step(neutral, 11)),
      "color-text-faint": restingInk(step(neutral, 10)),

      "color-focus-ring": focusRing(solid, text, backgrounds),
      "color-control-edge": controlEdge(text, canvas, backgrounds),
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
      "color-negative-wash": wash("negative"),
      "color-warning-wash": wash("warning"),
      "color-success-wash": wash("success"),
      "color-info-wash": wash("info"),

      "color-cta": solid,
      "color-cta-hover": step(accent, 10),
      "color-cta-text": onSolid,
    },
  };
}
