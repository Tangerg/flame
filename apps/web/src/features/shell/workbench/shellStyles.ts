import * as stylex from "@stylexjs/stylex";
import { color, layout, leading, radius, space, surface } from "@flame/ui/tokens.stylex";
export const shellStyles = stylex.create({
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
  overlay: { position: "absolute", inset: 0, display: "flex", flexDirection: "column" },
  anchor: { position: "relative", minHeight: 0, flex: 1 },

  spacer: { minWidth: space.s4, flex: 1 },
  toEnd: { marginInlineStart: "auto" },
  // Shares SystemMessage's icon size and gap so every banner's text starts on one line.
  bannerCard: {
    display: "grid",
    gridTemplateColumns: "auto minmax(0, 1fr) auto",
    alignItems: "flex-start",
    gap: layout.contentGap,
    borderRadius: radius.lg,
    backgroundColor: surface.negativeWash,
    paddingInline: space.s3,
    paddingBlock: space.s2_5,
  },
  bannerBody: {
    whiteSpace: "pre-wrap",
    overflowWrap: "break-word",
    color: color.fgSoft,
    lineHeight: leading.body,
  },
  // The row hangs by the small buttons' inset, so the leading action's glyph or label
  // starts on the banner text and its fill bleeds.
  bannerActions: {
    marginTop: space.s1_5,
    marginInlineStart: `calc(-1 * ${space.s2})`,
    display: "flex",
    flexWrap: "wrap",
    alignItems: "center",
    gap: space.s1_5,
  },
  selectable: { userSelect: "text", overflowWrap: "anywhere" },
});
