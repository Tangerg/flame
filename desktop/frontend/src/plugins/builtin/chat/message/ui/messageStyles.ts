import * as stylex from "@stylexjs/stylex";
import { color, leading, motion, radius, space, surface, weight } from "@/styles/tokens.stylex";

/**
 * The four distances a seam between two render units can take.
 *
 * `renderUnitRhythm` decides WHICH seam a pair makes; this decides what that seam is worth.
 * Two owners, one fact each — the application knows the relationship, the view knows the step.
 */
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
  /** `anywhere` because it may be a path with nothing to break at. */
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

  // `minWidth: 0` so it can give way inside the row's trailing slot, which a locale can
  // overfill: "Eingabe erforderlich" is twice the width of "Needs input". The truncation goes
  // on the TEXT beside the dot rather than on this box, because a `StatusDot` paints its pulse
  // outside its own bounds and clipping here cut the halo off every running row.
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
    paddingInline: space.s3,
    paddingBlock: space.s2,
  },
  delegatedBody: {
    minWidth: 0,
    textWrap: "pretty",
    lineHeight: leading.prose,
    color: color.fgSoft,
  },
  delegatedBubble: {
    borderRadius: radius.card,
    backgroundColor: surface.sunken,
    paddingInline: space.s3,
    paddingBlock: space.s2,
    color: color.fg,
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
  },
  // A hidden action bar is out of the tab order too, not merely transparent: an action that is
  // not offered yet must not be reachable by keyboard either. `reveal.shown` is the third state
  // and lives in the design system, because hovering to reveal is not this row's invention.
  actionsHidden: { visibility: "hidden", opacity: 0 },
  actionsPinned: { opacity: 1 },
  actionsOutdentStart: { marginLeft: "calc((var(--control-height-sm) - var(--icon-sm)) / -2)" },
  actionsOutdentEnd: { marginRight: "calc((var(--control-height-sm) - var(--icon-sm)) / -2)" },
  unselectable: { userSelect: "none" },
});
