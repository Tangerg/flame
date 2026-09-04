import { describe, expect, it } from "vitest";
import { iconScaleCssVariables } from "./iconScale";
import { UI_FONT_SIZE_MAX_PX, UI_FONT_SIZE_MIN_PX } from "./typography";

const CAP_PX = 1.5;

/** What the eye judges: the attribute is read inside Lucide's 24-unit viewBox. */
function renderedStroke(vars: Readonly<Record<string, string>>, size: string): number {
  const box = Number.parseFloat(vars[`--icon-${size}`]!);
  return Number(vars[`--icon-stroke-${size}`]!) * (box / 24);
}

describe("icon scale", () => {
  it("leaves the small steps drawing at Lucide's own weight", () => {
    const vars = iconScaleCssVariables(14);

    for (const size of ["xs", "sm", "md"]) {
      expect(vars[`--icon-stroke-${size}`]).toBe("2");
    }
  });

  // The defect this exists for: at 28px the proportional rule asks for 2.33, and at the
  // largest UI text for 3 — a heavier icon family sitting beside the 12px ones.
  it("stops the stroke growing once a line stops reading as drawn", () => {
    const vars = iconScaleCssVariables(14);

    expect(renderedStroke(vars, "lg")).toBeCloseTo(CAP_PX, 2);
    expect(renderedStroke(vars, "xl")).toBeCloseTo(CAP_PX, 2);
  });

  it("holds the cap across every UI text size a person can choose", () => {
    for (let base = UI_FONT_SIZE_MIN_PX; base <= UI_FONT_SIZE_MAX_PX; base += 1) {
      const vars = iconScaleCssVariables(base);
      for (const size of ["xs", "sm", "md", "lg", "xl"]) {
        expect(renderedStroke(vars, size)).toBeLessThanOrEqual(CAP_PX + 0.001);
      }
    }
  });

  it("never asks for more than Lucide draws", () => {
    const vars = iconScaleCssVariables(UI_FONT_SIZE_MIN_PX);

    for (const size of ["xs", "sm", "md", "lg", "xl"]) {
      expect(Number(vars[`--icon-stroke-${size}`])).toBeLessThanOrEqual(2);
    }
  });
});
