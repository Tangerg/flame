import * as stylex from "@stylexjs/stylex";
import { motion } from "@flame/ui/tokens.stylex";
export const chevron = stylex.create({
  base: {
    flexShrink: 0,
    transitionProperty: "rotate",
    transitionDuration: motion.fast,
    transitionTimingFunction: motion.easeState,
  },
  shut: { rotate: "-90deg" },
});
