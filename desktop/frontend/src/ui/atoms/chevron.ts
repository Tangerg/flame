import * as stylex from "@stylexjs/stylex";

export const chevron = stylex.create({
  base: { flexShrink: 0, transitionProperty: "rotate" },
  shut: { rotate: "-90deg" },
});
