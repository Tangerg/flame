import * as stylex from "@stylexjs/stylex";
import { color, leading, motion, space, weight } from "@/styles/tokens.stylex";

export const seamStep = stylex.create({
  none: {},
  tight: { marginTop: space.s1_5 },
  close: { marginTop: space.s3 },
  apart: { marginTop: space.s4 },
  wide: { marginTop: space.s5 },
});

export const messageStyles = stylex.create({
  cardClip: { overflow: "hidden" },
  cardHead: { paddingInline: space.s4, paddingTop: space.s4, paddingBottom: space.s3 },
  cardHeadTight: {
    display: "flex",
    alignItems: "flex-start",
    justifyContent: "space-between",
    gap: space.s3,
    paddingInline: space.s4,
    paddingTop: space.s4,
    paddingBottom: space.s2,
  },
  cardBody: {
    display: "flex",
    flexDirection: "column",
    gap: space.s2,
    paddingInline: space.s4,
    paddingBottom: space.s2,
  },
  cardActions: {
    display: "flex",
    flexWrap: "wrap",
    alignItems: "center",
    justifyContent: "flex-end",
    gap: space.s2,
    paddingInline: space.s4,
    paddingTop: space.s2,
    paddingBottom: space.s4,
  },
  cardPrompt: {
    marginTop: space.s2,
    textWrap: "pretty",
    overflowWrap: "anywhere",
    fontWeight: weight.medium,
    lineHeight: leading.body,
    color: color.fg,
  },
  cardPromptFlush: {
    minWidth: 0,
    textWrap: "pretty",
    overflowWrap: "anywhere",
    fontWeight: weight.medium,
    lineHeight: leading.body,
    color: color.fg,
  },
  quote: { whiteSpace: "pre-wrap", lineHeight: leading.prose, color: color.fgMuted },
  identity: {
    display: "flex",
    minWidth: 0,
    alignItems: "center",
    gap: space.s2,
    lineHeight: leading.body,
    color: color.fgMuted,
  },

  statusWord: {
    display: "inline-flex",
    minWidth: 0,
    alignItems: "center",
    gap: space.s1,
    fontWeight: weight.medium,
  },

  body: { minWidth: 0, textWrap: "pretty", lineHeight: leading.prose, color: color.fg },
  bubble: {
    maxWidth: "70%",
    backgroundColor: "var(--app-user-message-surface)",
    paddingInline: space.s4,
    paddingBlock: space.s2_5,
  },
  column: {
    position: "relative",
    display: "flex",
    minWidth: 0,
    flexDirection: "column",
    gap: space.s2,
  },
  columnUser: { alignItems: "flex-end" },
  actions: {
    display: "flex",
    flexShrink: 0,
    transitionProperty: "opacity, visibility",
    transitionDuration: motion.fast,
    transitionTimingFunction: motion.easeState,
  },
  actionsHidden: { visibility: "hidden", opacity: 0 },
  actionsPinned: { opacity: 1 },
  actionsOutdentStart: { marginLeft: "calc((var(--control-height-sm) - var(--icon-sm)) / -2)" },
  actionsOutdentEnd: { marginRight: "calc((var(--control-height-sm) - var(--icon-sm)) / -2)" },
  unselectable: { userSelect: "none" },
});
