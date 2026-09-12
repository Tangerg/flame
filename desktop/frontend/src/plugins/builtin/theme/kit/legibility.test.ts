import { describe, expect, it } from "vitest";
import {
  WCAG_AA_NON_TEXT,
  WCAG_AA_TEXT,
  contrastRatio,
  inkOnFill,
  legibleMix,
  mixOklab,
} from "./legibility";

describe("contrastRatio", () => {
  it("spans black to white at the ratio the criterion names", () => {
    expect(contrastRatio("#000000", "#ffffff")).toBeCloseTo(21, 5);
    expect(contrastRatio("#ffffff", "#ffffff")).toBeCloseTo(1, 5);
  });

  it("does not care which way round the pair is given", () => {
    expect(contrastRatio("#3574f0", "#ffffff")).toBeCloseTo(
      contrastRatio("#ffffff", "#3574f0"),
      10,
    );
  });

  it("is the linearised measure, not perceived brightness", () => {
    expect(contrastRatio("#ffe066", "#ffffff")).toBeLessThan(1.6);
    expect(contrastRatio("#ffe066", "#000000")).toBeGreaterThan(13);
  });
});

describe("inkOnFill", () => {
  it.each(["#2b5fd0", "#115beb"])("keeps the theme's own choice when it reads (%s)", (fill) => {
    expect(contrastRatio(fill, "#ffffff")).toBeGreaterThanOrEqual(WCAG_AA_TEXT);
    expect(inkOnFill("#ffffff", fill, WCAG_AA_TEXT)).toBe("#ffffff");
  });

  it("flips even on a fill that only just fails", () => {
    expect(contrastRatio("#3574f0", "#ffffff")).toBeLessThan(WCAG_AA_TEXT);
    expect(inkOnFill("#ffffff", "#3574f0", WCAG_AA_TEXT)).toBe("#000000");
  });

  it.each(["#ffe066", "#a8e6a3", "#f5c2e7"])("overrules a pale fill (%s)", (fill) => {
    expect(inkOnFill("#ffffff", fill, WCAG_AA_TEXT)).toBe("#000000");
    expect(contrastRatio(fill, inkOnFill("#ffffff", fill, WCAG_AA_TEXT))).toBeGreaterThanOrEqual(
      WCAG_AA_TEXT,
    );
  });

  it("leaves a dark fill on the light pole", () => {
    expect(inkOnFill("#ffffff", "#111111", WCAG_AA_TEXT)).toBe("#ffffff");
  });

  it("picks the further pole when the declared ink fails", () => {
    const fill = "#008080";
    const ink = inkOnFill("#ffffff", fill, WCAG_AA_TEXT);
    expect(contrastRatio(fill, ink)).toBeGreaterThanOrEqual(contrastRatio(fill, "#ffffff"));
  });

  it("answers differently for a mark than for a label", () => {
    expect(inkOnFill("#ffffff", "#3574f0", WCAG_AA_TEXT)).toBe("#000000");
    expect(inkOnFill("#ffffff", "#3574f0", WCAG_AA_NON_TEXT)).toBe("#ffffff");
  });
});

describe("mixOklab", () => {
  it("returns each end at its own extreme", () => {
    expect(mixOklab("#3574f0", "#ffffff", 100).toLowerCase()).toBe("#3574f0");
    expect(mixOklab("#3574f0", "#ffffff", 0).toLowerCase()).toBe("#ffffff");
  });

  it("moves toward the ink as its share grows", () => {
    const steps = [0, 25, 50, 75, 100].map((pct) =>
      contrastRatio(mixOklab("#000000", "#ffffff", pct), "#ffffff"),
    );
    for (let i = 1; i < steps.length; i += 1) expect(steps[i]!).toBeGreaterThan(steps[i - 1]!);
  });
});

describe("legibleMix", () => {
  it("raises a rung that does not read", () => {
    const floor = 34;
    const chosen = legibleMix("#e3e5e9", "#1d1f23", "#1d1f23", floor, WCAG_AA_TEXT);
    expect(chosen).toBeGreaterThan(floor);
    expect(contrastRatio(mixOklab("#e3e5e9", "#1d1f23", chosen), "#1d1f23")).toBeGreaterThanOrEqual(
      WCAG_AA_TEXT,
    );
  });

  it("leaves a rung that already reads exactly where the design put it", () => {
    const floor = 88;
    expect(legibleMix("#000000", "#ffffff", "#ffffff", floor, WCAG_AA_TEXT)).toBe(floor);
  });

  it("gives up at the ink itself when even that does not read", () => {
    const ink = "#5a5a5a";
    const fill = "#202020";
    expect(contrastRatio(ink, fill)).toBeLessThan(WCAG_AA_TEXT);
    expect(legibleMix(ink, fill, fill, 28, WCAG_AA_TEXT)).toBe(100);
  });
});
