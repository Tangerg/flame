import * as stylex from "@stylexjs/stylex";
import { radius, surface } from "@flame/ui/tokens.stylex";

export const artifactFrame = stylex.create({
  frame: {
    marginBlock: "calc(var(--md-space) * 5)",
    borderRadius: radius.lg,
    borderWidth: "var(--control-edge-width)",
    borderStyle: "solid",
    borderColor: surface.field,
    backgroundColor: surface.card,
  },
});
