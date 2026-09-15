import { describe, expect, it } from "vitest";
import { disclosureExitTransition, disclosureTransition, selectionTransition } from "./motion";

const seconds = (transition: { duration?: number }): number => transition.duration ?? 0;

describe("motion ladder", () => {
  it("leaves faster than it arrives", () => {
    expect(
      seconds(disclosureExitTransition),
      "an exit that matches its entrance makes the reader wait on an animation with nothing left to say",
    ).toBeLessThan(seconds(disclosureTransition));
  });

  it("spends the rung below rather than a number of its own", () => {
    expect(
      seconds(disclosureExitTransition),
      "a duration the ladder cannot express is one nobody can hold",
    ).toBe(seconds(selectionTransition));
  });
});
