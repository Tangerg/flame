import * as stylex from "@stylexjs/stylex";
import { color, leading, radius, space, surface, weight } from "@/styles/tokens.stylex";

/**
 * The shell's own arrangement: the panes that hold the transcript and the dock.
 *
 * `flex min-h-0 flex-1 flex-col` appears eleven times across this plugin, and the `min-h-0`
 * is the load-bearing half — without it a scrolling child stretches its parent instead of
 * scrolling, which is the one mistake this shape exists to stop repeating.
 */
export const shellStyles = stylex.create({
  pane: { display: "flex", minHeight: 0, flex: 1, flexDirection: "column" },
  paneAnchored: {
    position: "relative",
    display: "flex",
    minHeight: 0,
    flex: 1,
    flexDirection: "column",
  },
  paneNarrow: {
    position: "relative",
    display: "flex",
    minHeight: 0,
    minWidth: 0,
    flex: 1,
    flexDirection: "column",
  },
  fill: { position: "absolute", inset: 0, display: "flex", flexDirection: "column" },
  anchor: { position: "relative", minHeight: 0, flex: 1 },
  column: { display: "flex", flexDirection: "column" },
  line: { display: "flex", alignItems: "center", gap: space.s2 },
  lineTight: { display: "flex", alignItems: "center", gap: space.s1_5 },
  min: { minWidth: 0 },
  hold: { flexShrink: 0 },
  truncate: { overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" },
  muted: { color: color.fgMuted },
  faint: { color: color.fgFaint },
  soft: { color: color.fgSoft },
  ink: { color: color.fg },
  accent: { color: color.accent },
  negative: { color: color.negative },
  success: { color: color.success },
  warning: { color: color.warning },
  strong: { fontWeight: weight.semibold },
  wrapText: { whiteSpace: "pre-wrap", overflowWrap: "break-word" },
  pretty: { textWrap: "pretty" },
  balance: { textWrap: "balance" },

  /** Takes the bar's spare width so a title beside it truncates instead of pushing. */
  spacer: { minWidth: space.s4, flex: 1 },
  /** A banner between two messages in the transcript, which owns the gap on both sides. */
  banner: { marginBlock: space.s2_5 },
  bannerCard: {
    display: "grid",
    gridTemplateColumns: "auto 1fr auto",
    alignItems: "flex-start",
    gap: space.s2_5,
    borderRadius: radius.lg,
    borderWidth: "1px",
    borderStyle: "solid",
    borderColor: "var(--color-negative-edge)",
    backgroundColor: surface.card,
    paddingInline: space.s3,
    paddingBlock: space.s2_5,
  },
  bannerHead: {
    marginBottom: space.s0_5,
    display: "flex",
    flexWrap: "wrap",
    alignItems: "baseline",
    columnGap: space.s2,
  },
  bannerBody: {
    whiteSpace: "pre-wrap",
    overflowWrap: "break-word",
    color: color.fgSoft,
    lineHeight: leading.body,
  },
  bannerActions: {
    marginTop: space.s1_5,
    display: "flex",
    flexWrap: "wrap",
    alignItems: "center",
    gap: space.s1_5,
  },
  /** Machine text the reader copies out, so it opts back into selection. */
  selectable: { userSelect: "text", overflowWrap: "anywhere" },
});
