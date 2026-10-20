import { describe, expect, it } from "vitest";
import { SCHEME_ICON, depthStep, inkTracksLadder } from "@/features/theme/kit/tokens";
describe("the ink ladder", () => {
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
