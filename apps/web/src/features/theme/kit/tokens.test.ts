import { describe, expect, it } from "vitest";
import {
  SCHEME_ICON,
  depthStep,
  inkTracksLadder,
  inkTrackingMix,
} from "@/features/theme/kit/tokens";
describe("the ink ladder", () => {
  it.each(
    (["light", "dark"] as const).flatMap((scheme) =>
      [0, 25, 50, 100].map((contrast) => ({ scheme, contrast })),
    ),
  )(
    "keeps numeric ink qualification aligned with $scheme CSS at $contrast",
    ({ scheme, contrast }) => {
      const css = inkTracksLadder(scheme, "#60666d");
      const formula = css.match(/- ([\d.]+)%\) \* ([\d.]+)/)!;
      const expected = Math.max(
        0,
        (Number.parseFloat(depthStep(scheme, contrast)) - Number(formula[1])) * Number(formula[2]),
      );
      expect(inkTrackingMix(scheme, contrast)).toBe(expected);
    },
  );

  it("anchors the ink to the step its own scheme opens on", () => {
    expect(inkTracksLadder("light", "#60666d")).toBe(
      "color-mix(in oklab, var(--color-text) max(0%, calc((var(--depth-step) - 4%) * 3)), #60666d)",
    );
    expect(inkTracksLadder("dark", "#9fa4a9")).toContain("var(--depth-step) - 8%");
  });

  it("doubles the dark step so both schemes separate equally", () => {
    expect(depthStep("light", 25)).toBe("4%");
    expect(depthStep("dark", 25)).toBe("8%");
  });

  it("maps dark/light to moon/sun", () => {
    expect(SCHEME_ICON.dark).toBe("moon");
    expect(SCHEME_ICON.light).toBe("sun");
  });
});
