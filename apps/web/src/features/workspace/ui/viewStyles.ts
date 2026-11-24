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
    display: "grid",
    flex: 1,
    gridTemplateColumns: "minmax(0, 1fr)",
    rowGap: space.s0_5,
    columnGap: space.s1_5,
    paddingBlockStart: space.s1,
    minWidth: 0,
  },
  // The icon sits in its own column so the metadata lines start under the name, not the icon.
  headerIdentityWithIcon: { gridTemplateColumns: "auto minmax(0, 1fr)" },
  headerIcon: { display: "flex", alignItems: "center", height: "1lh", color: color.fgFaint },
  headerName: {
    gridColumn: -2,
    minWidth: 0,
    color: color.fg,
    fontWeight: weight.medium,
    overflowWrap: "anywhere",
  },
  headerMetadata: { gridColumn: -2, minWidth: 0, overflowWrap: "anywhere" },
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
