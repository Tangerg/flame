import { describe, expect, it } from "vitest";
import type { Scheme } from "@/lib/appearance";
import { WCAG_AA_NON_TEXT, WCAG_AA_TEXT, contrastRatio } from "./legibility";
import { toOklch } from "./oklab";
import { paletteThemeContribution, SCHEME_BASE, type PaletteTheme } from "./palette";
import { solidFill } from "./scale";

const theme = (scheme: Scheme, palette: PaletteTheme["palette"] = {}): PaletteTheme => ({
  id: `test-${scheme}`,
  label: "Test",
  scheme,
  palette,
});

const tokensOf = (subject: PaletteTheme, accent = "#0485f7", contrast = 25) =>
  paletteThemeContribution(subject, accent, contrast).tokens!;

const restingInk = (token: string) => token.match(/(#[0-9a-f]{6})\)$/i)?.[1] ?? token;

const ACCENTS = ["#0485f7", "#7f52ff", "#21a179", "#e8590c", "#ffe066", "#101828"];

const APPEARANCES: ReadonlyArray<[string, PaletteTheme]> = [
  ["built-in light", theme("light")],
  ["built-in dark", theme("dark")],
  ["custom dark", theme("dark", { background: "#0f1117", foreground: "#e6e8ee" })],
  ["custom light", theme("light", { background: "#fdf6e3", foreground: "#073642" })],
];

describe("palette themes", () => {
  it("emit one role vocabulary for built-in, custom and partial palettes", () => {
    const keys = (subject: PaletteTheme) => Object.keys(tokensOf(subject)).sort();
    const builtin = keys(theme("light"));
    for (const [, subject] of APPEARANCES) expect(keys(subject)).toEqual(builtin);
    expect(builtin).toEqual(
      expect.arrayContaining([
        "color-accent",
        "color-accent-text",
        "color-accent-strong",
        "color-accent-line",
        "color-border-strong",
        "tone-accent",
        "tone-negative",
      ]),
    );
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

  it("tint only the built-in neutrals toward the accent hue", () => {
    const hue = (color: string) => toOklch(color).h;
    const blue = tokensOf(theme("light"), "#0485f7")["color-surface"]!;
    const green = tokensOf(theme("light"), "#21a179")["color-surface"]!;
    expect(Math.abs(hue(blue) - hue("#0485f7"))).toBeLessThan(15);
    expect(Math.abs(hue(green) - hue("#21a179"))).toBeLessThan(15);

    const custom = theme("light", { background: "#fdf6e3", foreground: "#073642" });
    expect(tokensOf(custom, "#0485f7")["color-surface"]).toBe(
      tokensOf(custom, "#21a179")["color-surface"],
    );
    expect(tokensOf(theme("light"))["color-text"]).not.toBe(SCHEME_BASE.light.foreground);
  });

  it("order the neutral planes and hairlines the way their roles read", () => {
    for (const scheme of ["light", "dark"] as const) {
      const t = tokensOf(theme(scheme));
      const lightness = (name: string) => toOklch(t[name]!).l;
      const away = scheme === "light" ? -1 : 1;
      expect(away * (lightness("color-sunken") - lightness("color-surface"))).toBeGreaterThan(0);
      expect(away * (lightness("color-surface") - lightness("color-bg"))).toBeGreaterThanOrEqual(0);
      const edge = (name: string) => contrastRatio(t[name]!, t["color-bg"]!);
      expect(edge("color-divider")).toBeLessThan(edge("color-border"));
      expect(edge("color-border")).toBeLessThan(edge("color-border-strong"));
    }
  });

  it("hold the accent hue from tint to text", () => {
    for (const scheme of ["light", "dark"] as const) {
      const t = tokensOf(theme(scheme), "#7f52ff");
      const solidHue = toOklch(t["color-accent"]!).h;
      for (const name of ["color-accent-line", "color-accent-text", "color-accent-strong"])
        expect({ name, drift: Math.abs(toOklch(t[name]!).h - solidHue) < 15 }).toEqual({
          name,
          drift: true,
        });
    }
  });

  it.each(
    ["#0485f7", "#7f52ff", "#21a179", "#e8590c", "#7dd3fc", "#ffe066"].flatMap((accent) =>
      (["light", "dark"] as const).map((scheme) => [accent, scheme] as const),
    ),
  )("keep the primary label readable at rest and on hover (%s, %s)", (accent, scheme) => {
    const t = tokensOf(theme(scheme), accent);
    const label = t["color-cta-text"]!;
    expect(contrastRatio(label, t["color-cta"]!)).toBeGreaterThanOrEqual(WCAG_AA_TEXT);
    expect(contrastRatio(label, t["color-cta-hover"]!)).toBeGreaterThanOrEqual(WCAG_AA_TEXT);
    expect(t["color-cta-hover"]).not.toBe(t["color-cta"]);
    expect(Math.abs(toOklch(t["color-cta-hover"]!).h - toOklch(t["color-cta"]!).h)).toBeLessThan(
      15,
    );
  });

  it("separate the planes further as the contrast preference rises", () => {
    const edge = (contrast: number) => {
      const t = tokensOf(theme("light"), "#0485f7", contrast);
      return contrastRatio(t["color-border"]!, t["color-bg"]!);
    };
    expect(edge(0)).toBeLessThan(edge(25));
    expect(edge(25)).toBeLessThan(edge(100));
  });

  describe.each(APPEARANCES)("measured pairs in %s", (_label, subject) => {
    it.each(ACCENTS.flatMap((accent) => [0, 25, 100].map((contrast) => [accent, contrast])))(
      "read with accent %s at contrast %d",
      (accent, contrast) => {
        const t = tokensOf(subject, accent as string, contrast as number);
        const planes = ["color-bg", "color-surface", "color-sunken", "color-elevated"].map(
          (name) => t[name]!,
        );
        const reads = (ink: string, required: number) =>
          planes.every((plane) => contrastRatio(ink, plane) >= required);

        for (const name of [
          "color-text",
          "color-text-soft",
          "color-text-muted",
          "color-text-faint",
        ])
          expect({ name, reads: reads(restingInk(t[name]!), WCAG_AA_TEXT) }).toEqual({
            name,
            reads: true,
          });
        for (const name of [
          "color-accent-text",
          "color-accent-strong",
          "color-negative",
          "color-warning",
          "color-success",
          "color-info",
        ])
          expect({ name, reads: reads(t[name]!, WCAG_AA_TEXT) }).toEqual({ name, reads: true });
        expect(reads(t["color-accent-strong"]!, 7)).toBe(true);

        expect(contrastRatio(t["color-cta-text"]!, t["color-cta"]!)).toBeGreaterThanOrEqual(
          WCAG_AA_TEXT,
        );
        expect(
          contrastRatio(t["color-text-on-accent"]!, t["color-accent"]!),
        ).toBeGreaterThanOrEqual(WCAG_AA_TEXT);
        expect(contrastRatio(t["color-focus-ring"]!, t["color-bg"]!)).toBeGreaterThanOrEqual(
          WCAG_AA_NON_TEXT,
        );
        expect(contrastRatio(t["color-control-edge"]!, t["color-bg"]!)).toBeGreaterThanOrEqual(
          WCAG_AA_NON_TEXT,
        );
      },
    );
  });
});
