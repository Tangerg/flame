import { describe, expect, it } from "vitest";
import { fixtureMotionScale } from "./motion";

describe("the visual fixture motion query", () => {
  it.each([
    [null, 0],
    ["off", 0],
    ["full", 1],
  ] as const)("maps %j to motion scale %s", (value, scale) => {
    expect(fixtureMotionScale(value)).toBe(scale);
  });

  // An exact message also proves the rejected URL content is never echoed.
  it.each(["on", "", "Full", "true", "1", " full", "<secret-token>"])("rejects %j", (value) => {
    expect(() => fixtureMotionScale(value)).toThrow(
      new Error('visual fixture motion must be "full" or "off", or omitted for off'),
    );
  });
});
