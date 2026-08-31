import { describe, expect, it } from "vitest";
import {
  UI_FONT_SIZE_DEFAULT_PX,
  UI_FONT_SIZE_MAX_PX,
  UI_FONT_SIZE_MIN_PX,
  normalizeUiFontSize,
} from "./typography";

describe("normalizeUiFontSize", () => {
  it("falls back to the default for absent or non-finite input", () => {
    expect(normalizeUiFontSize(null)).toBe(UI_FONT_SIZE_DEFAULT_PX);
    expect(normalizeUiFontSize(undefined)).toBe(UI_FONT_SIZE_DEFAULT_PX);
    expect(normalizeUiFontSize(Number.NaN)).toBe(UI_FONT_SIZE_DEFAULT_PX);
  });

  it("clamps and rounds into the supported range", () => {
    expect(normalizeUiFontSize(2)).toBe(UI_FONT_SIZE_MIN_PX);
    expect(normalizeUiFontSize(99)).toBe(UI_FONT_SIZE_MAX_PX);
    expect(normalizeUiFontSize(12.6)).toBe(13);
  });
});
