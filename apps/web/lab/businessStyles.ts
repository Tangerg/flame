import * as stylex from "@stylexjs/stylex";
import { color, leading, space } from "@flame/ui/tokens.stylex";

export const businessStyles = stylex.create({
  stack: { display: "flex", minWidth: 0, flexDirection: "column", gap: space.s4 },
  rows: { display: "flex", minWidth: 0, flexDirection: "column", gap: space.s1 },
  row: { display: "flex", minWidth: 0, flexWrap: "wrap", alignItems: "center", gap: space.s3 },
  caption: { color: color.fgMuted, overflowWrap: "anywhere" },
  reading: {
    whiteSpace: "pre-wrap",
    overflowWrap: "anywhere",
    lineHeight: leading.body,
    paddingBlock: space.s3,
  },
  catalog: { minHeight: 0, height: "22rem" },
  popup: {
    display: "flex",
    flexDirection: "column",
    gap: space.s1,
    padding: space.s2,
    maxWidth: "24rem",
  },
  composer: { containerType: "inline-size", containerName: "composer", minWidth: 0 },
  compactComposer: { width: "12rem", maxWidth: "100%" },
});
