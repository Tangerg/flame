import { describe, expect, it } from "vitest";
import { accentTintedNeutral, hexToOklch, neutralChromaFactor, oklchToHex } from "./accentTint";

const LIGHT = {
  surface: { l: 97.3, c: 0.006 },
  elevated: { l: 98.4, c: 0.008 },
  sunken: { l: 95.4, c: 0.016 },
} as const;

const BLUE = "#2b5fd0";
const PURPLE = "#7f52ff";
const GREEN = "#21a179";
const ORANGE = "#e8590c";

const chromaOf = (hex: string) => hexToOklch(hex).c;

const REFERENCE = BLUE;

const family = (accent: string, tint?: "off" | "soft" | "standard") =>
  Object.fromEntries(
    Object.entries(LIGHT).map(([key, step]) => [
      key,
      accentTintedNeutral(accent, REFERENCE, step, tint),
    ]),
  ) as Record<keyof typeof LIGHT, string>;

describe("hexToOklch / oklchToHex", () => {
  it("round-trips a colour through OKLCH", () => {
    for (const hex of [BLUE, PURPLE, GREEN, ORANGE, "#ffffff", "#000000", "#808080"]) {
      expect(oklchToHex(hexToOklch(hex))).toBe(hex);
    }
  });

  it("gives up chroma rather than hue when a request leaves sRGB", () => {
    const asked = { l: 98, c: 0.4, h: 255 };
    const got = hexToOklch(oklchToHex(asked));
    expect(got.c).toBeLessThan(asked.c);
    expect(Math.abs(got.h - asked.h)).toBeLessThan(4);
    expect(Math.abs(got.l - asked.l)).toBeLessThan(1.5);
  });
});

describe("neutralChromaFactor", () => {
  it("is exactly 1 for the accent the family was tuned against", () => {
    expect(neutralChromaFactor(hexToOklch(BLUE).c, chromaOf(REFERENCE), "standard")).toBeCloseTo(
      1,
      2,
    );
  });

  it("scales with how much colour the accent actually has", () => {
    const vivid = neutralChromaFactor(hexToOklch(PURPLE).c, chromaOf(REFERENCE), "standard");
    const muted = neutralChromaFactor(hexToOklch(GREEN).c, chromaOf(REFERENCE), "standard");
    expect(vivid).toBeGreaterThan(1);
    expect(muted).toBeLessThan(1);
  });

  it("caps a neon accent", () => {
    expect(
      neutralChromaFactor(hexToOklch("#0000ff").c, chromaOf(REFERENCE), "standard"),
    ).toBeLessThanOrEqual(1.5);
  });

  it("reaches zero for an accent with no hue to borrow", () => {
    for (const achromatic of ["#000000", "#ffffff", "#7a7a7a"]) {
      expect(
        neutralChromaFactor(hexToOklch(achromatic).c, chromaOf(REFERENCE), "standard"),
      ).toBeLessThan(1e-4);
    }
  });

  it("halves at `soft` and vanishes at `off`", () => {
    const c = hexToOklch(BLUE).c;
    expect(neutralChromaFactor(c, chromaOf(REFERENCE), "soft")).toBeCloseTo(0.5, 2);
    expect(neutralChromaFactor(c, chromaOf(REFERENCE), "off")).toBe(0);
  });
});

describe("accentTintedNeutral", () => {
  it("leaves the default accent's family exactly where it was", () => {
    expect(family(BLUE)).toEqual({
      surface: "#f4f6fa",
      elevated: "#f7faff",
      sunken: "#eaf0fb",
    });
  });

  it("turns the family onto the accent's hue", () => {
    const warm = family(ORANGE);
    const accentHue = hexToOklch(ORANGE).h;
    for (const hex of Object.values(warm)) {
      expect(Math.abs(hexToOklch(hex).h - accentHue)).toBeLessThan(6);
    }
  });

  it("goes grey for a grey accent instead of red", () => {
    for (const achromatic of ["#000000", "#ffffff", "#7a7a7a"]) {
      for (const hex of Object.values(family(achromatic))) {
        expect(chromaOf(hex)).toBeLessThan(0.002);
      }
    }
  });

  it("tints a vivid accent more than a muted one at the same step", () => {
    expect(chromaOf(family(PURPLE).sunken)).toBeGreaterThan(chromaOf(family(BLUE).sunken));
    expect(chromaOf(family(GREEN).sunken)).toBeLessThan(chromaOf(family(BLUE).sunken));
  });

  it("collapses the whole family to neutral at `off`", () => {
    for (const hex of Object.values(family(PURPLE, "off"))) {
      expect(chromaOf(hex)).toBeLessThan(0.002);
    }
  });

  it("keeps each step's lightness whatever the accent", () => {
    for (const accent of [BLUE, PURPLE, GREEN, ORANGE, "#000000"]) {
      const tinted = family(accent);
      for (const [key, step] of Object.entries(LIGHT)) {
        expect(hexToOklch(tinted[key as keyof typeof LIGHT]).l).toBeCloseTo(step.l, 0);
      }
    }
  });
});
