import * as stylex from "@stylexjs/stylex";
import { leading, weight } from "@flame/ui/tokens.stylex";
export const chatStyles = stylex.create({
  centre: { textAlign: "center" },
  medium: { fontWeight: weight.medium },
  clampOne: {
    display: "-webkit-box",
    WebkitBoxOrient: "vertical",
    WebkitLineClamp: 1,
    overflow: "hidden",
  },
  clampThree: {
    display: "-webkit-box",
    WebkitBoxOrient: "vertical",
    WebkitLineClamp: 3,
    overflow: "hidden",
  },
  bodyLeading: { lineHeight: leading.body },
  snugLeading: { lineHeight: leading.snug },
});
