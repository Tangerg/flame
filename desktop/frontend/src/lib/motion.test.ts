import { describe, expect, it } from "vitest";
import { visualStyleMotion, motionScale } from "./appearance";
import { disclosureExitTransition, disclosureTransition } from "./motion";

const seconds = (transition: { duration?: number }): number => transition.duration ?? 0;

describe("motion ladder", () => {
  it("leaves faster than it arrives", () => {
    expect(
      seconds(disclosureExitTransition),
      "an exit that matches its entrance makes the reader wait on an animation with nothing left to say",
    ).toBeLessThan(seconds(disclosureTransition));
  });

  it("uses the theme fast rung for disclosure exits", () => {
    expect(
      seconds(disclosureExitTransition),
      "a duration the ladder cannot express is one nobody can hold",
    ).toBe((visualStyleMotion().fastMs / 1000) * motionScale());
  });
});
