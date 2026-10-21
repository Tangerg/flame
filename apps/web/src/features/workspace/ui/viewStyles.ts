import * as stylex from "@stylexjs/stylex";
import { color, leading, layout, space, surface, weight } from "@flame/ui/tokens.stylex";
export const viewStyles = stylex.create({
  contentHeader: {
    display: "flex",
    flexDirection: "column",
    gap: layout.controlGap,
    flexShrink: 0,
    minWidth: 0,
    paddingInline: layout.compactGutter,
    paddingBlock: layout.controlGap,
  },
  headerHeading: {
    display: "flex",
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

export const codeStyles = stylex.create({
  sheet: { paddingBlock: space.s2, fontFamily: "var(--font-mono)", lineHeight: leading.relaxed },
  gutter: { textAlign: "right", color: color.fgFaint, userSelect: "none" },
  wrap: { minWidth: 0, whiteSpace: "pre-wrap", overflowWrap: "anywhere" },
  lineRow: {
    display: "grid",
    alignItems: "flex-start",
    gap: space.s2,
    paddingInline: space.s3,
  },
  targetLine: { backgroundColor: surface.accentWash },
});
