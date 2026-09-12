import * as stylex from "@stylexjs/stylex";
import { describe, expect, it } from "vitest";
import { transcriptTurnContentVisibility } from "./transcriptTurnContentVisibility";

describe("transcript turn content visibility", () => {
  it("keeps historical turns eligible for off-screen rendering skips", () => {
    expect(stylex.props(transcriptTurnContentVisibility(false)).className).toBeTruthy();
  });

  it("always renders the tail turn which owns current outcome and HITL controls", () => {
    expect(transcriptTurnContentVisibility(true)).toBeUndefined();
  });
});
