import * as stylex from "@stylexjs/stylex";
import { color, space, surface } from "@flame/ui/tokens.stylex";

export const foundationStyles = stylex.create({
  stack: { display: "flex", flexDirection: "column", gap: space.s4, minWidth: 0 },
  row: { display: "flex", alignItems: "center", flexWrap: "wrap", gap: space.s3, minWidth: 0 },
  pair: {
    display: "flex",
    flexDirection: "column",
    alignItems: "flex-start",
    gap: space.s2,
    minWidth: 0,
  },
  label: { color: color.fgMuted },
  value: { color: color.fgMuted, fontVariantNumeric: "tabular-nums" },
  media: { backgroundColor: surface.mediaScrim, padding: space.s3 },
  popup: { padding: space.s5, maxWidth: "24rem" },
  scroll: { maxHeight: "24rem", overflowY: "auto", minWidth: 0 },
  image: { display: "flex", minHeight: "22rem", height: "22rem" },
  list: {
    display: "flex",
    flexDirection: "column",
    gap: space.s2,
    margin: 0,
    paddingInlineStart: space.s5,
  },
  area: { maxHeight: "12rem" },
});
