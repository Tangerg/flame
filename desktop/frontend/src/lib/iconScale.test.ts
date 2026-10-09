import { describe, expect, it } from "vitest";
import { iconScaleCssVariables, iconSizePx, type IconSize } from "./iconScale";
import { UI_FONT_SIZE_MAX_PX, UI_FONT_SIZE_MIN_PX } from "./typography";

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
      const variables = iconScaleCssVariables(font);
      for (const [size, pixels] of Object.entries(expected)) {
        expect(iconSizePx(size as IconSize, font)).toBe(pixels);
        expect(variables[`--icon-${size}`]).toBe(`${pixels}px`);
      }
    }
  });

  it("normalizes missing and out-of-range font preferences", () => {
    expect(iconSizePx("sm", null)).toBe(14);
    expect(iconSizePx("xs", 0)).toBe(9);
    expect(iconSizePx("xl", 100)).toBe(36);
  });
});
