import { describe, expect, it } from "vitest";
import { colord } from "colord";
import { type Scheme } from "@/lib/appearance";
import { collapse, declaredInCss } from "@/test/stylesheet";
import { visualStyleTokens } from "@/features/theme/visualStyles/tokens";
import { depthStep } from "@/features/theme/kit/tokens";
import {
  WCAG_AA_NON_TEXT,
  WCAG_AA_TEXT,
  contrastRatio,
  type ContrastColor,
  type RgbColor,
} from "@/features/theme/kit/legibility";
import { toOklch } from "@/features/theme/kit/oklab";
import {
  paletteThemeContribution,
  SCHEME_BASE,
  type PaletteTheme,
} from "@/features/theme/kit/palette";
import { solidFill } from "@/features/theme/kit/scale";
import { paperTextureBackgrounds } from "@/features/theme/kit/paperTexture";
const theme = (scheme: Scheme, palette: PaletteTheme["palette"] = {}): PaletteTheme => ({
  id: `test-${scheme}`,
  label: "Test",
  scheme,
  palette,
});

const tokensOf = (subject: PaletteTheme, accent = "#0485f7", contrast = 25) =>
  paletteThemeContribution(subject, accent, contrast).tokens!;

const restingInk = (token: string) => token.match(/(#[0-9a-f]{6})\)$/i)?.[1] ?? token;

function paintedPlane(
  tokens: Record<string, string>,
  paint: string | undefined,
  plane: ContrastColor,
  washStep = 0,
): RgbColor {
  if (paint?.startsWith("#")) return colord(paint).toRgb();
  const resolved =
    paint === undefined
      ? undefined
      : collapse(paint).replace(
          /calc\(var\(--wash-step\) \* ([\d.]+)\)/g,
          (_expression, factor) => `${washStep * Number(factor)}%`,
        );
  const parts = resolved?.match(
    /^color-mix\(in srgb,\s*var\(--([a-z-]+)\)\s+([\d.]+)%,\s*(transparent|var\(--([a-z-]+)\))\)$/,
  );
  if (!parts) throw new Error(`Unmeasured paint: ${paint}`);
  const ink = colord(tokens[parts[1]!]!).toRgb();
  const backing = parts[3] === "transparent" ? plane : tokens[parts[4]!]!;
  const base = typeof backing === "string" ? colord(backing).toRgb() : backing;
  const alpha = Number(parts[2]) / 100;
  return {
    r: ink.r * alpha + base.r * (1 - alpha),
    g: ink.g * alpha + base.g * (1 - alpha),
    b: ink.b * alpha + base.b * (1 - alpha),
  };
}

function washedPlane(tokens: Record<string, string>, tone: string, plane: ContrastColor): RgbColor {
  const property = `color-${tone}-wash`;
  return paintedPlane(tokens, tokens[property] ?? declaredInCss(`--${property}`), plane);
}

const NEUTRAL_ROLES = [
  "color-bg",
  "color-drawer",
  "color-surface",
  "color-elevated",
  "color-sunken",
  "color-text",
  "color-text-bright",
  "color-text-soft",
  "color-text-muted",
  "color-text-faint",
  "color-control-edge",
  "color-divider",
  "color-border",
  "color-border-strong",
];

const neutralsOf = (subject: PaletteTheme, accent: string, contrast: number) => {
  const t = tokensOf(subject, accent, contrast);
  return Object.fromEntries(NEUTRAL_ROLES.map((name) => [name, t[name]!]));
};

const ACCENTS = ["#fb6e30", "#0485f7", "#7f52ff", "#21a179", "#e8590c", "#ffe066", "#101828"];

const APPEARANCES: ReadonlyArray<[string, PaletteTheme]> = [
  ["built-in light", theme("light")],
  ["built-in dark", theme("dark")],
  ["custom dark", theme("dark", { background: "#0f1117", foreground: "#e6e8ee" })],
  ["custom light", theme("light", { background: "#fdf6e3", foreground: "#073642" })],
];

describe.each(APPEARANCES)("paper reading contrast in %s", (_label, subject) => {
  it.each(["off", "xuan", "cotton", "grain"] as const)(
    "keeps text and focus readable on %s hover and selection states",
    (paperTexture) => {
      for (const contrast of [0, 25, 100]) {
        for (const accent of ACCENTS) {
          const t = paletteThemeContribution(subject, accent, contrast, {
            paperTexture,
            paperTextureStrength: 5,
          }).tokens!;
          const effective: Record<string, string> = { ...t, ...visualStyleTokens({}) };
          const planes: ContrastColor[] = [
            "color-bg",
            "color-drawer",
            "color-surface",
            "color-elevated",
            "color-sunken",
          ].map((name) => t[name]!);
          planes.push(
            ...paperTextureBackgrounds(paperTexture, 5, subject.scheme, [
              t["color-bg"]!,
              t["color-drawer"]!,
              t["color-surface"]!,
            ]),
          );
          const washStep = Math.max(3, Number.parseFloat(depthStep("light", contrast)));
          for (const state of ["wash-hover", "wash-selected", "wash-selected-hover"]) {
            const paint = effective[state] ?? declaredInCss(`--${state}`);
            for (const plane of planes) {
              const fill = paintedPlane(t, paint, plane, washStep);
              for (const name of [
                "color-text",
                "color-text-soft",
                "color-text-muted",
                "color-text-faint",
                "color-accent-text",
                "color-accent-strong",
                "color-negative",
                "color-warning",
                "color-success",
                "color-info",
              ])
                expect(
                  contrastRatio(restingInk(t[name]!), fill),
                  `${name}, ${state}, ${subject.scheme}, ${paperTexture}, ${contrast}, ${accent}`,
                ).toBeGreaterThanOrEqual(WCAG_AA_TEXT);
              for (const name of ["color-focus-ring", "color-control-edge"])
                expect(contrastRatio(t[name]!, fill), `${name}, ${state}`).toBeGreaterThanOrEqual(
                  WCAG_AA_NON_TEXT,
                );
            }
          }
        }
      }
    },
  );

  it.each(["off", "xuan", "cotton", "grain"] as const)(
    "keeps semantic text readable over its painted %s feedback",
    (paperTexture) => {
      for (const contrast of [0, 25, 100]) {
        for (const accent of ACCENTS) {
          const t = paletteThemeContribution(subject, accent, contrast, {
            paperTexture,
            paperTextureStrength: 5,
          }).tokens!;
          const planes: ContrastColor[] = [
            "color-bg",
            "color-drawer",
            "color-surface",
            "color-elevated",
            "color-sunken",
          ].map((name) => t[name]!);
          planes.push(
            ...paperTextureBackgrounds(paperTexture, 5, subject.scheme, [
              t["color-bg"]!,
              t["color-drawer"]!,
              t["color-surface"]!,
            ]),
          );
          for (const tone of ["accent", "negative", "warning", "success", "info"]) {
            const ink = t[tone === "accent" ? "color-accent-text" : `color-${tone}`]!;
            for (const plane of planes)
              expect(
                contrastRatio(ink, washedPlane(t, tone, plane)),
                `${tone} on ${plane}, ${subject.scheme}, ${paperTexture}, ${contrast}, ${accent}`,
              ).toBeGreaterThanOrEqual(WCAG_AA_TEXT);
          }
        }
      }
    },
  );

  it("restores every plain token when paper is Off", () => {
    expect(
      paletteThemeContribution(subject, "#fb6e30", 25, {
        paperTexture: "off",
        paperTextureStrength: 5,
      }).tokens,
    ).toEqual(tokensOf(subject, "#fb6e30", 25));
  });

  it.each(["xuan", "cotton", "grain"] as const)(
    "keeps text and focus readable on the strongest %s pixels",
    (paperTexture) => {
      for (const paperTextureStrength of [1, 3, 5]) {
        for (const contrast of [0, 25, 100]) {
          for (const accent of ACCENTS) {
            const plain = tokensOf(subject, accent, contrast);
            const t = paletteThemeContribution(subject, accent, contrast, {
              paperTexture,
              paperTextureStrength,
            }).tokens!;
            for (const name of [
              "color-bg",
              "color-drawer",
              "color-surface",
              "color-elevated",
              "color-sunken",
              "color-text",
              "color-cta",
              "color-cta-hover",
            ])
              expect(t[name]).toBe(plain[name]);
            const planes: ContrastColor[] = [
              "color-bg",
              "color-drawer",
              "color-surface",
              "color-elevated",
              "color-sunken",
            ].map((name) => t[name]!);
            planes.push(
              ...paperTextureBackgrounds(paperTexture, paperTextureStrength, subject.scheme, [
                t["color-bg"]!,
                t["color-drawer"]!,
                t["color-surface"]!,
              ]),
            );
            for (const name of [
              "color-text",
              "color-text-soft",
              "color-text-muted",
              "color-text-faint",
              "color-accent-text",
              "color-accent-strong",
              "color-negative",
              "color-warning",
              "color-success",
              "color-info",
            ])
              for (const plane of planes)
                expect(
                  contrastRatio(restingInk(t[name]!), plane),
                  `${name} on ${plane}, strength ${paperTextureStrength}, contrast ${contrast}, accent ${accent}`,
                ).toBeGreaterThanOrEqual(WCAG_AA_TEXT);
            for (const name of ["color-focus-ring", "color-control-edge"])
              for (const plane of planes)
                expect(
                  contrastRatio(t[name]!, plane),
                  `${name} on ${plane}`,
                ).toBeGreaterThanOrEqual(WCAG_AA_NON_TEXT);
          }
        }
      }
    },
  );
});

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

  it.each(["light", "dark"] as const)(
    "derive the built-in %s neutrals from the scheme canvas and ink alone",
    (scheme) => {
      const achromatic = (color: string) => {
        expect(restingInk(color)).toMatch(/^#[0-9a-f]{6}$/i);
        const [r, g, b] = [1, 3, 5].map((at) => restingInk(color).slice(at, at + 2));
        return r === g && g === b;
      };
      for (const contrast of [0, 25, 100]) {
        const reference = neutralsOf(theme(scheme), ACCENTS[0]!, contrast);
        expect(reference["color-bg"]).toBe(SCHEME_BASE[scheme].background);
        expect(reference["color-text"]).toBe(SCHEME_BASE[scheme].foreground);
        for (const [name, color] of Object.entries(reference))
          expect({ name, achromatic: achromatic(color) }).toEqual({ name, achromatic: true });
        for (const accent of ACCENTS)
          expect(neutralsOf(theme(scheme), accent, contrast)).toEqual(reference);
      }
      expect(tokensOf(theme(scheme), "#fb6e30")["color-accent"]).not.toBe(
        tokensOf(theme(scheme), "#0485f7")["color-accent"],
      );
    },
  );

  it.each([
    ...APPEARANCES.slice(2),
    ["partial canvas", theme("light", { background: "#fdf6e3" })],
    ["partial ink", theme("dark", { foreground: "#e6e8ee" })],
    ["declared accent", theme("dark", { accent: "#b4004e" })],
  ] as const)("keep the %s neutrals independent of the preferred accent", (_label, subject) => {
    const reference = neutralsOf(subject, ACCENTS[0]!, 25);
    expect(reference["color-bg"]).toBe(
      subject.palette.background ?? SCHEME_BASE[subject.scheme].background,
    );
    expect(reference["color-text"]).toBe(
      subject.palette.foreground ?? SCHEME_BASE[subject.scheme].foreground,
    );
    for (const accent of ACCENTS) expect(neutralsOf(subject, accent, 25)).toEqual(reference);
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
    ["#fb6e30", "#0485f7", "#7f52ff", "#21a179", "#e8590c", "#7dd3fc", "#ffe066"].flatMap(
      (accent) => (["light", "dark"] as const).map((scheme) => [accent, scheme] as const),
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

  it("keep a seed too light for a white mark as its own solid under dark ink", () => {
    for (const seed of ["#fb6e30", "#f59e0b", "#ffe066"]) {
      expect(solidFill(seed)).toBe(seed);
      for (const scheme of ["light", "dark"] as const)
        expect(tokensOf(theme(scheme), seed)["color-text-on-accent"]).toBe("#000000");
    }
    for (const seed of ["#0485f7", "#21a179", "#e5484d", "#30a46c", "#0090ff"]) {
      const solid = solidFill(seed);
      expect(contrastRatio(solid, "#ffffff")).toBeGreaterThanOrEqual(WCAG_AA_TEXT);
      expect(toOklch(solid).l).toBeLessThan(toOklch(seed).l);
      expect(Math.abs(toOklch(solid).h - toOklch(seed).h)).toBeLessThan(15);
    }
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
        const planes = [
          "color-bg",
          "color-drawer",
          "color-surface",
          "color-sunken",
          "color-elevated",
        ].map((name) => t[name]!);
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
        for (const name of ["color-focus-ring", "color-control-edge"])
          expect({ name, reads: reads(t[name]!, WCAG_AA_NON_TEXT) }).toEqual({ name, reads: true });
      },
    );
  });
});
