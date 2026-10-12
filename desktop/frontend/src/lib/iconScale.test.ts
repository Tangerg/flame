import { describe, expect, it } from "vitest";
import {
  productIconScaleCssVariables,
  productIconSizePx,
  type ProductIconSize,
} from "./productIconScale";
import { UI_FONT_SIZE_MAX_PX, UI_FONT_SIZE_MIN_PX } from "@flame/ui/typography";

describe("icon sizes", () => {
  it("derives actual pixels and CSS layout variables from the same font policy", () => {
    for (let font = UI_FONT_SIZE_MIN_PX; font <= UI_FONT_SIZE_MAX_PX; font++) {
      const expected = {
        xs: font - 2,
        sm: font,
        md: font + 2,
        lg: font + 6,
        xl: font * 2,
        composer: 16,
      };
      const variables = productIconScaleCssVariables(font);
      for (const [size, pixels] of Object.entries(expected)) {
        expect(productIconSizePx(size as ProductIconSize, font)).toBe(pixels);
        expect(variables[`--icon-${size}`]).toBe(`${pixels}px`);
      }
    }
  });

  it("normalizes missing and out-of-range font preferences", () => {
    expect(productIconSizePx("sm", null)).toBe(14);
    expect(productIconSizePx("xs", 0)).toBe(9);
    expect(productIconSizePx("xl", 100)).toBe(36);
  });
});
