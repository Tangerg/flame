import { describe, expect, it } from "vitest";
import { WCAG_AA_NON_TEXT, WCAG_AA_TEXT, contrastRatio, inkOnFill } from "./legibility";

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
