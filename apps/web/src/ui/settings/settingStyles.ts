import * as stylex from "@stylexjs/stylex";
import {
  color,
  leading,
  layout,
  motion,
  radius,
  space,
  surface,
  weight,
} from "@flame/ui/tokens.stylex";
export const settingStyles = stylex.create({
  stack: { display: "flex", flexDirection: "column", gap: layout.fieldGap, minWidth: 0 },

  split: {
    display: "flex",
    flexWrap: "wrap",
    alignItems: "center",
    justifyContent: "space-between",
    gap: layout.contentGap,
    minWidth: 0,
  },
  lineWide: {
    display: "flex",
    flexWrap: "wrap",
    alignItems: "center",
    gap: layout.contentGap,
    minWidth: 0,
  },
  lineWrap: {
    display: "flex",
    flexWrap: "wrap",
    alignItems: "center",
    gap: layout.controlGap,
    minWidth: 0,
  },
  end: { display: "flex", justifyContent: "flex-end" },

  label: { color: color.fg, fontWeight: weight.medium },
  hint: { color: color.fgMuted, lineHeight: leading.body },
  hintSpaced: { marginTop: space.s1, color: color.fgMuted, lineHeight: leading.body },

  caption: { marginBottom: space.s1_5, color: color.fgMuted, fontWeight: weight.medium },
  inline: { display: "inline-flex", alignItems: "center", gap: space.s1 },
  selfEnd: { alignSelf: "flex-end" },
  spin: { animation: motion.spin },

  sunkenRow: {
    display: "flex",
    alignItems: "center",
    justifyContent: "space-between",
    gap: layout.contentGap,
    flexWrap: "wrap",
    minWidth: 0,
    borderRadius: radius.card,
    backgroundColor: {
      default: surface.sunken,
      ":hover": { "@media (hover: hover)": surface.sunkenHover },
    },
    paddingInline: space.s3,
    paddingBlock: space.s1_5,
    transitionProperty: "background-color",
    transitionTimingFunction: motion.easeState,
  },
});
