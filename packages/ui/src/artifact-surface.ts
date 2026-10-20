import * as stylex from "@stylexjs/stylex";
import { radius, surface } from "./tokens.stylex";
export const ARTIFACT_SURFACE = stylex.create({
  face: {
    borderRadius: radius.lg,
    borderWidth: "var(--control-edge-width)",
    borderStyle: "solid",
    borderColor: surface.field,
    backgroundColor: surface.card,
  },
});
