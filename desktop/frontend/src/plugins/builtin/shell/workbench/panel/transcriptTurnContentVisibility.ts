import * as stylex from "@stylexjs/stylex";
import type { StyleXStyles } from "@stylexjs/stylex";

const offscreen = stylex.create({
  skip: { contentVisibility: "auto", containIntrinsicSize: "auto 220px" },
});

export function transcriptTurnContentVisibility(isLast: boolean): StyleXStyles | undefined {
  return isLast ? undefined : offscreen.skip;
}
