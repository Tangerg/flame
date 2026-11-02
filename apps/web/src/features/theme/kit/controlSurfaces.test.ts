import { describe, expect, it } from "vitest";
import { interpolate, rgbColor } from "@/lib/color";
import { type Scheme } from "@/lib/appearance";
import { mixOklab } from "@/features/theme/kit/oklab";
import { depthStep } from "@/features/theme/kit/tokens";
import {
  contrastRatio,
  relativeLuminance,
  type ContrastColor,
} from "@/features/theme/kit/legibility";
import { paletteThemePreview, type PaletteTheme } from "@/features/theme/kit/palette";

function theme(scheme: Scheme, palette: PaletteTheme["palette"] = {}): PaletteTheme {
  return { id: scheme, label: scheme, scheme, palette };
}

const MID_DARK = theme("dark", { background: "#505967", foreground: "#f0f4fb" });
const MID_LIGHT = theme("light", { background: "#afa996", foreground: "#111111" });
const SUBJECTS = [
  theme("light"),
  theme("dark"),
  theme("light", { background: "#fdf6e3", foreground: "#073642" }),
  theme("dark", { background: "#0f1117", foreground: "#e6e8ee" }),
];

function paintedInk(css: string, primary: string, scheme: Scheme, contrast: number): ContrastColor {
  const match = css.match(/- ([\d.]+)%\) \* ([\d.]+)\)\), (#[\da-f]{6})\)/i);
  if (!match) throw new Error(`Unrecognized foreground paint: ${css}`);
  const depth = Number.parseFloat(depthStep(scheme, contrast));
  const percent = Math.max(0, (depth - Number(match[1])) * Number(match[2]));
  const ink = rgbColor(interpolate([match[3]!, primary], "oklab")(percent / 100))!;
  return { r: ink.r * 255, g: ink.g * 255, b: ink.b * 255 };
}

const cases = [
  ...SUBJECTS.flatMap((subject, index) =>
    [0, 25, 100].flatMap((contrast) =>
      (["off", "xuan"] as const).map((paperTexture) => ({
        label: `${index}/${subject.scheme}/${contrast}/${paperTexture}`,
        subject,
        contrast,
        paperTexture,
      })),
    ),
  ),
  ...[0, 25].map((contrast) => ({
    label: `mid-dark/${contrast}`,
    subject: MID_DARK,
    contrast,
    paperTexture: "off" as const,
  })),
  ...[0, 25, 100].map((contrast) => ({
    label: `mid-light/${contrast}`,
    subject: MID_LIGHT,
    contrast,
    paperTexture: "off" as const,
  })),
];

describe("neutral control reading planes", () => {
  it.each(cases)(
    "qualifies ordinary text and focus on $label",
    ({ subject, contrast, paperTexture }) => {
      const preview = paletteThemePreview(subject, "#0485f7", contrast, {
        paperTexture,
        paperTextureStrength: 5,
      });
      expect(preview.textContrast).toBeGreaterThanOrEqual(4.5);
      const t = preview.contribution.tokens!;
      const primary = t["color-text"]!;
      const soft = paintedInk(t["color-text-soft"]!, primary, subject.scheme, contrast);
      const muted = paintedInk(t["color-text-muted"]!, primary, subject.scheme, contrast);
      for (const role of ["color-surface-2", "color-surface-3"]) {
        const fill = t[role]!;
        expect(fill).toMatch(/^#[\da-f]{6}$/i);
        const inks = role === "color-surface-2" ? [primary, soft, muted] : [primary, soft];
        for (const ink of inks) expect(contrastRatio(ink, fill)).toBeGreaterThanOrEqual(4.5);
        for (const edge of ["color-focus-ring", "color-control-edge"])
          expect(contrastRatio(t[edge]!, fill)).toBeGreaterThanOrEqual(3);
      }
    },
  );

  it.each(["light", "dark"] as const)("retains the qualified %s default depth", (scheme) => {
    const t = paletteThemePreview(theme(scheme), "#fb6e30", 25).contribution.tokens!;
    const preferred = Number.parseFloat(depthStep(scheme, 25));
    expect(t["color-surface-2"]).toBe(mixOklab(t["color-text"]!, t["color-surface"]!, preferred));
    expect(t["color-surface-3"]).toBe(
      mixOklab(t["color-text"]!, t["color-surface"]!, preferred * 2),
    );
  });

  it("keeps resting and hovered surfaces distinct in the observed custom theme", () => {
    const t = paletteThemePreview(MID_DARK, "#0485f7", 25).contribution.tokens!;
    const base = relativeLuminance(t["color-surface"]!);
    const resting = relativeLuminance(t["color-surface-2"]!);
    const hovered = relativeLuminance(t["color-surface-3"]!);
    expect(resting).toBeGreaterThan(base);
    expect(hovered).toBeGreaterThan(resting);
  });

  it("keeps an already invalid saved pair explicit and adds no extra depth", () => {
    const preview = paletteThemePreview(MID_DARK, "#0485f7", 100);
    expect(preview.textContrast).toBeLessThan(4.5);
    const t = preview.contribution.tokens!;
    expect(t["color-surface-2"]).toBe(t["color-surface"]);
    expect(t["color-surface-3"]).toBe(t["color-surface"]);
  });
});
