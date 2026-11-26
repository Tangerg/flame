import * as stylex from "@stylexjs/stylex";
import { edge, radius, surface } from "./tokens.stylex";
export const ARTIFACT_SURFACE = stylex.create({
  face: {
    borderRadius: radius.lg,
    borderWidth: edge.control,
    borderStyle: "solid",
    borderColor: surface.field,
    backgroundColor: surface.card,
  },
});
