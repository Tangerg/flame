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
      // The editorial steps land on exactly the pixel values they used to carry as literals.
      // That is the point of deriving their ratios from this base: the default renders
      // unchanged and only the ends of the range move.
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

  // Reading text may never end up at or below the chrome around it — that
  // collapse is exactly what the step was added to prevent, and rounding plus the
  // ceiling are where it could happen quietly.
  it("keeps reading text above the chrome across the whole base range", () => {
    for (let base = UI_FONT_SIZE_MIN_PX; base <= UI_FONT_SIZE_MAX_PX; base += 1) {
      const ladder = uiTypeLadder(base);
      expect(ladder.prose).toBeGreaterThan(ladder["ui-md"]);
    }
  });

  // A heading may never reach the size of the text it heads. As fixed pixel values the
  // editorial steps did exactly that at the top of the range: at base 18 `display-sm` was 18px
  // over 21px prose, and markdown's own h3 was 17px over the same 21px. `markdown-h5` is the
  // exception BY DESIGN — it is a bold label whose weight, not its size, carries the level —
  // so what it has to keep is its place under h3, not a size above prose.
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
    // Ratio alone would put ui-2xs at 8px here; the floor holds it at 9.
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
