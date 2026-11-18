import { describe, expect, it } from "vitest";
import { type Scheme } from "@/lib/appearance";
import { WCAG_AA_TEXT, contrastRatio } from "@/features/theme/kit/legibility";
import {
  paletteThemeContribution,
  paletteThemePreview,
  type PaletteTheme,
} from "@/features/theme/kit/palette";
import { solidFill } from "@/features/theme/kit/scale";
const theme = (scheme: Scheme, palette: PaletteTheme["palette"] = {}): PaletteTheme => ({
  id: `test-${scheme}`,
  label: "Test",
  scheme,
  palette,
});

const tokensOf = (subject: PaletteTheme, accent = "#0485f7", contrast = 25) =>
  paletteThemeContribution(subject, accent, contrast).tokens!;

describe("palette themes", () => {
  it("reports unreadable declared colors without replacing them", () => {
    const preview = paletteThemePreview(
      theme("light", { background: "#808080", foreground: "#808080" }),
      "#fb6e30",
      25,
    );
    expect(preview.textContrast).toBeCloseTo(1, 10);
    expect(preview.contribution.tokens).toMatchObject({
      "color-bg": "#808080",
      "color-text": "#808080",
    });
  });

  it("includes derived reading planes in the preview's minimum text contrast", () => {
    const preview = paletteThemePreview(
      theme("light", { background: "#ffffff", foreground: "#767676" }),
      "#fb6e30",
      25,
    );
    expect(contrastRatio("#767676", "#ffffff")).toBeGreaterThanOrEqual(WCAG_AA_TEXT);
    expect(preview.textContrast).toBeLessThan(WCAG_AA_TEXT);
  });

  it("includes the existing worst-case paper paint in the preview", () => {
    const subject = theme("light", { background: "#ffffff", foreground: "#686868" });
    expect(paletteThemePreview(subject, "#fb6e30", 25).textContrast).toBeGreaterThanOrEqual(
      WCAG_AA_TEXT,
    );
    const paper = paletteThemePreview(subject, "#fb6e30", 25, {
      paperTexture: "xuan",
      paperTextureStrength: 5,
    });
    expect(paper.textContrast).toBeLessThan(WCAG_AA_TEXT);
    expect(paper.contribution.tokens?.["color-text"]).toBe("#686868");
  });

  it("keep a declared canvas and ink exactly, and a declared accent over the preference", () => {
    const declared = tokensOf(
      theme("dark", { background: "#0f1117", foreground: "#e6e8ee", accent: "#b4004e" }),
      "#3574f0",
    );
    expect(declared["color-bg"]).toBe("#0f1117");
    expect(declared["color-text"]).toBe("#e6e8ee");
    expect(declared["color-accent"]).toBe(solidFill("#b4004e"));
  });
});
