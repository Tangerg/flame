import * as stylex from "@stylexjs/stylex";
import { leading, radius, space, surface } from "./tokens.stylex";
export const WELL_SURFACE = stylex.create({
  face: {
    borderRadius: radius.sm,
    backgroundColor: surface.sunken,
    paddingInline: space.s3,
    paddingBlock: space.s2_5,
    fontFamily: "var(--font-mono)",
    lineHeight: leading.relaxed,
  },
});
