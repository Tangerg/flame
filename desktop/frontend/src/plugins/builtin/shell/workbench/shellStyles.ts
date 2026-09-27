import * as stylex from "@stylexjs/stylex";
import { color, leading, radius, space, surface } from "@/styles/tokens.stylex";

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
  overlay: { position: "absolute", inset: 0, display: "flex", flexDirection: "column" },
  anchor: { position: "relative", minHeight: 0, flex: 1 },
  balance: { textWrap: "balance" },

  spacer: { minWidth: space.s4, flex: 1 },
  banner: { marginBlock: space.s2_5 },
  bannerCard: {
    display: "grid",
    gridTemplateColumns: "auto 1fr auto",
    alignItems: "flex-start",
    gap: space.s2_5,
    borderRadius: radius.lg,
    borderWidth: "var(--control-edge-width)",
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
  selectable: { userSelect: "text", overflowWrap: "anywhere" },
});
