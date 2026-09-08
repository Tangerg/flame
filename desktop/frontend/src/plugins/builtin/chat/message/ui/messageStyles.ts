import * as stylex from "@stylexjs/stylex";
import { color, leading, motion, space, weight } from "@/styles/tokens.stylex";

/**
 * A card in the transcript: an approval, a question, a compaction notice.
 *
 * The three of them share an inset ladder — the head sits deeper than the body, and the
 * actions deeper still — and one line: the thing being asked, which wraps anywhere because
 * it may be a path or a command with no spaces to break at.
 */
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
  /** What is being asked. `anywhere` because it may be a path with nothing to break at. */
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
  /** A quoted body the agent produced: reasoning, a compaction summary, a settled answer. */
  quote: { whiteSpace: "pre-wrap", lineHeight: leading.prose, color: color.fgMuted },
  identity: {
    display: "flex",
    minWidth: 0,
    alignItems: "center",
    gap: space.s2,
    lineHeight: leading.body,
    color: color.fgMuted,
  },

  /** A dot and the word beside it. The ink is the status's, and comes from `toneInk`. */
  statusWord: {
    display: "inline-flex",
    alignItems: "center",
    gap: space.s1,
    fontWeight: weight.medium,
  },

  /** The message body: one measure, one leading, and the bubble only if the reader wrote it. */
  body: { minWidth: 0, textWrap: "pretty", lineHeight: leading.prose, color: color.fg },
  bubble: {
    maxWidth: "70%",
    borderRadius: "var(--radius-bubble)",
    backgroundColor: "var(--color-user-message)",
    paddingInline: space.s3,
    paddingBlock: space.s2,
  },
  column: {
    position: "relative",
    display: "flex",
    minWidth: 0,
    flexDirection: "column",
    gap: space.s2,
  },
  columnUser: { alignItems: "flex-end" },
  /** The row of actions under a message. How visible it is, the three steps below say. */
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
  // The bar hangs half a control's overhang outside the text column, so the first GLYPH lines
  // up with the text edge rather than the button box around it.
  actionsOutdentStart: { marginLeft: "calc((var(--control-height-sm) - var(--icon-sm)) / -2)" },
  actionsOutdentEnd: { marginRight: "calc((var(--control-height-sm) - var(--icon-sm)) / -2)" },
  unselectable: { userSelect: "none" },
});
