import * as stylex from "@stylexjs/stylex";
import { color, leading, space, weight } from "@/styles/tokens.stylex";

/**
 * A card in the transcript: an approval, a question, a compaction notice.
 *
 * The three of them share an inset ladder — the head sits deeper than the body, and the
 * actions deeper still — and one line: the thing being asked, which wraps anywhere because
 * it may be a path or a command with no spaces to break at.
 */
export const messageStyles = stylex.create({
  clip: { overflow: "hidden" },
  head: { paddingInline: space.s4, paddingTop: space.s4, paddingBottom: space.s3 },
  headTight: {
    display: "flex",
    alignItems: "flex-start",
    justifyContent: "space-between",
    gap: space.s3,
    paddingInline: space.s4,
    paddingTop: space.s4,
    paddingBottom: space.s2,
  },
  body: {
    display: "flex",
    flexDirection: "column",
    gap: space.s2,
    paddingInline: space.s4,
    paddingBottom: space.s2,
  },
  actions: {
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
  prompt: {
    marginTop: space.s2,
    textWrap: "pretty",
    overflowWrap: "anywhere",
    fontWeight: weight.medium,
    lineHeight: leading.body,
    color: color.fg,
  },
  promptFlush: {
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
});
