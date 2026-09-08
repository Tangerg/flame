import * as stylex from "@stylexjs/stylex";
import { describe, expect, it } from "vitest";
import { transcriptTurnContentVisibility } from "./transcriptTurnContentVisibility";

describe("transcript turn content visibility", () => {
  // Asserting the class STRING is what let this rot: the previous spelling was two Tailwind
  // arbitrary-property classes, and the assertion kept passing for a whole release after the
  // utility that gave them meaning was gone. What the module owes its caller is a style that
  // resolves to something; whether that something still says `content-visibility` is a
  // question only the rendered document can answer, and `agentStates` now asks it.
  it("keeps historical turns eligible for off-screen rendering skips", () => {
    expect(stylex.props(transcriptTurnContentVisibility(false)).className).toBeTruthy();
  });

  it("always renders the tail turn which owns current outcome and HITL controls", () => {
    expect(transcriptTurnContentVisibility(true)).toBeUndefined();
  });
});
