import { describe, expect, it } from "vitest";
import { createAppearanceProjection } from "@/lib/appearance";
import { createMotion } from "@/lib/motion";
const seconds = (transition: { duration?: number }): number => transition.duration ?? 0;

describe("motion ladder", () => {
  it("leaves faster than it arrives", () => {
    const appearance = createAppearanceProjection();
    const { disclosureExitTransition, disclosureTransition } = createMotion(appearance);
    expect(
      seconds(disclosureExitTransition),
      "an exit that matches its entrance makes the reader wait on an animation with nothing left to say",
    ).toBeLessThan(seconds(disclosureTransition));
  });

  it("uses the theme fast rung for disclosure exits", () => {
    const appearance = createAppearanceProjection();
    const { disclosureExitTransition } = createMotion(appearance);
    expect(
      seconds(disclosureExitTransition),
      "a duration the ladder cannot express is one nobody can hold",
    ).toBe((appearance.read().motion.fastMs / 1000) * appearance.read().motionScale);
  });
});
