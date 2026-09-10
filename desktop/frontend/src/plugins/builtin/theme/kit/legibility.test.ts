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
  // The two anchors the criterion itself is defined against, so a wrong coefficient or a
  // missing linearisation shows up immediately rather than as a value that merely looks close.
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

  // `colord`'s `brightness()` is a perceptual weighting, not this: on the accent that shipped
  // it reads about 0.42 while the ratio against white is 5.6, and a threshold on the first
  // would put the ink on the wrong side.
  it("is the linearised measure, not perceived brightness", () => {
    expect(contrastRatio("#ffe066", "#ffffff")).toBeLessThan(1.6);
    expect(contrastRatio("#ffe066", "#000000")).toBeGreaterThan(13);
  });
});

describe("inkOnFill", () => {
  // The accents the product actually opens on, one per scheme.
  it.each(["#2b5fd0", "#115beb"])("keeps the theme's own choice when it reads (%s)", (fill) => {
    expect(contrastRatio(fill, "#ffffff")).toBeGreaterThanOrEqual(WCAG_AA_TEXT);
    expect(inkOnFill("#ffffff", fill, WCAG_AA_TEXT)).toBe("#ffffff");
  });

  // Worth pinning because it is the value the theme SPEC carries as its brand accent, used as
  // the tint reference rather than painted: white on it is 4.28, under the line. If it ever
  // became the live accent the ink would flip, and that is the rule working, not a regression.
  it("flips even on a fill that only just fails", () => {
    expect(contrastRatio("#3574f0", "#ffffff")).toBeLessThan(WCAG_AA_TEXT);
    expect(inkOnFill("#ffffff", "#3574f0", WCAG_AA_TEXT)).toBe("#000000");
  });

  // The accents that broke it, measured: white on either was under 2:1.
  it.each(["#ffe066", "#a8e6a3", "#f5c2e7"])("overrules a pale fill (%s)", (fill) => {
    expect(inkOnFill("#ffffff", fill, WCAG_AA_TEXT)).toBe("#000000");
    expect(contrastRatio(fill, inkOnFill("#ffffff", fill, WCAG_AA_TEXT))).toBeGreaterThanOrEqual(
      WCAG_AA_TEXT,
    );
  });

  it("leaves a dark fill on the light pole", () => {
    expect(inkOnFill("#ffffff", "#111111", WCAG_AA_TEXT)).toBe("#ffffff");
  });

  // A mid fill is the interesting case: neither pole is obviously right, and the answer has to
  // be the one further away rather than the one the theme happened to declare.
  it("picks the further pole when the declared ink fails", () => {
    const fill = "#008080";
    const ink = inkOnFill("#ffffff", fill, WCAG_AA_TEXT);
    expect(contrastRatio(fill, ink)).toBeGreaterThanOrEqual(contrastRatio(fill, "#ffffff"));
  });

  // The same fill, judged as a mark rather than as a label. `#3574f0` reads 4.28 against white:
  // under the line for text and over it for a graphic, so the two callers must not share a
  // threshold — judging a checkbox's mark as text would flip the shipped palette.
  it("answers differently for a mark than for a label", () => {
    expect(inkOnFill("#ffffff", "#3574f0", WCAG_AA_TEXT)).toBe("#000000");
    expect(inkOnFill("#ffffff", "#3574f0", WCAG_AA_NON_TEXT)).toBe("#ffffff");
  });
});

describe("mixOklab", () => {
  // The ends are the only points a mix cannot get wrong, so they are where a swapped argument
  // or an inverted weight shows up. That the MIDDLE matches the browser is pinned by
  // `visual/oklabPrediction.visual.spec.ts`, which paints it and reads the pixels back.
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
  // The case that started this: the product's own dark colours, handed to the derivation that
  // builds a custom palette. The designed rung reads 2.4:1 and has to be raised.
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

  // An ink the user chose too close to their own background cannot be rescued by mixing — the
  // most this can do is stop mixing. Stated as the property rather than a number: the first
  // pair tried here was `#909090`, which is 5.10:1 against that background, so a 93% mix
  // cleared it and the case was not the one it claimed to be.
  it("gives up at the ink itself when even that does not read", () => {
    const ink = "#5a5a5a";
    const fill = "#202020";
    expect(contrastRatio(ink, fill)).toBeLessThan(WCAG_AA_TEXT);
    expect(legibleMix(ink, fill, fill, 28, WCAG_AA_TEXT)).toBe(100);
  });
});
