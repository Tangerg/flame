import * as stylex from "@stylexjs/stylex";
import { color, layout, space, weight } from "@flame/ui/tokens.stylex";
export const viewStyles = stylex.create({
  contentHeader: {
    display: "flex",
    flexDirection: "row",
    flexWrap: "wrap",
    alignItems: "flex-start",
    gap: layout.controlGap,
    flexShrink: 0,
    minWidth: 0,
    paddingInline: layout.compactGutter,
    paddingBlock: layout.controlGap,
  },
  headerHeading: {
    display: "flex",
    flexGrow: 1,
    alignItems: "flex-start",
    gap: layout.controlGap,
    minWidth: 0,
  },
  headerIdentity: {
    display: "flex",
    flex: 1,
    flexDirection: "column",
    gap: space.s0_5,
    paddingBlockStart: space.s1,
    minWidth: 0,
  },
  headerName: { minWidth: 0, color: color.fg, fontWeight: weight.medium, overflowWrap: "anywhere" },
  headerMetadata: { minWidth: 0, overflowWrap: "anywhere" },
  headerControls: {
    display: "flex",
    flexGrow: 1,
    flexWrap: "wrap",
    alignItems: "center",
    gap: layout.controlGap,
    minWidth: 0,
  },
  headerActions: {
    display: "flex",
    flexWrap: "wrap",
    alignItems: "center",
    gap: layout.controlGap,
    minWidth: 0,
    marginInlineStart: "auto",
  },
});
