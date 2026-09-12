import { describe, expect, it } from "vitest";
import {
  UI_FONT_SIZE_DEFAULT_PX,
  UI_FONT_SIZE_MAX_PX,
  UI_FONT_SIZE_MIN_PX,
} from "@/lib/typography";
import { uiTypeLadder, uiTypeLadderCssVariables } from "./typeLadder";

describe("uiTypeLadder", () => {
  it("lands the default base on the whole-pixel grid", () => {
    expect(uiTypeLadder(UI_FONT_SIZE_DEFAULT_PX)).toEqual({
      "ui-2xs": 11,
      "ui-xs": 12,
      "ui-sm": 13,
      "ui-md": 14,
      prose: 16,
      code: 13,
      "markdown-h5": 15,
      "markdown-h3": 17,
      "display-sm": 18,
      "display-md": 20,
      "display-lg": 24,
    });
  });

  it("never inverts a step across the whole base range", () => {
    for (let base = UI_FONT_SIZE_MIN_PX; base <= UI_FONT_SIZE_MAX_PX; base += 1) {
      const ladder = uiTypeLadder(base);
      const ascending = [ladder["ui-2xs"], ladder["ui-xs"], ladder["ui-sm"], ladder["ui-md"]];
      expect(ascending).toEqual([...ascending].sort((a, b) => a - b));
      expect(ladder.code).toBeLessThanOrEqual(ladder["ui-md"]);
    }
  });

  it("keeps reading text above the chrome across the whole base range", () => {
    for (let base = UI_FONT_SIZE_MIN_PX; base <= UI_FONT_SIZE_MAX_PX; base += 1) {
      const ladder = uiTypeLadder(base);
      expect(ladder.prose).toBeGreaterThan(ladder["ui-md"]);
    }
  });

  it("keeps every heading above the reading text it heads", () => {
    for (let base = UI_FONT_SIZE_MIN_PX; base <= UI_FONT_SIZE_MAX_PX; base += 1) {
      const ladder = uiTypeLadder(base);
      for (const step of ["markdown-h3", "display-sm", "display-md", "display-lg"] as const) {
        expect(ladder[step]).toBeGreaterThan(ladder.prose);
      }
      expect(ladder["markdown-h5"]).toBeLessThan(ladder["markdown-h3"]);
    }
  });

  it("keeps the small end legible when the base shrinks", () => {
    expect(uiTypeLadder(UI_FONT_SIZE_MIN_PX)["ui-2xs"]).toBe(9);
  });

  it("normalizes the base before deriving", () => {
    expect(uiTypeLadder(null)).toEqual(uiTypeLadder(UI_FONT_SIZE_DEFAULT_PX));
    expect(uiTypeLadder(1000)).toEqual(uiTypeLadder(UI_FONT_SIZE_MAX_PX));
  });
});

describe("uiTypeLadderCssVariables", () => {
  it("emits every ladder step as a px custom property", () => {
    expect(uiTypeLadderCssVariables(UI_FONT_SIZE_DEFAULT_PX)).toEqual({
      "--fs-ui-2xs": "11px",
      "--fs-ui-xs": "12px",
      "--fs-ui-sm": "13px",
      "--fs-ui-md": "14px",
      "--fs-prose": "16px",
      "--fs-code": "13px",
      "--fs-markdown-h5": "15px",
      "--fs-markdown-h3": "17px",
      "--fs-display-sm": "18px",
      "--fs-display-md": "20px",
      "--fs-display-lg": "24px",
    });
  });
});
