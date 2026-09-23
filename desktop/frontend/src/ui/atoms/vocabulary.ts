import * as stylex from "@stylexjs/stylex";
import { color, space, weight } from "@/styles/tokens.stylex";

export const vocab = stylex.create({
  fill: { minWidth: 0, flex: 1 },
  grow: { flex: 1 },
  hold: { flexShrink: 0 },
  min: { minWidth: 0 },
  column: { display: "flex", flexDirection: "column" },
  line: { display: "flex", alignItems: "center", gap: space.s2 },
  lineTight: { display: "flex", alignItems: "center", gap: space.s1_5 },
  firstLine: { display: "flex", height: "1lh", flexShrink: 0, alignItems: "center" },

  afterLine: { marginTop: space.s1_5 },

  truncate: { overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" },
  wrapText: { whiteSpace: "pre-wrap", overflowWrap: "break-word" },
  pretty: { textWrap: "pretty" },
  figures: { fontVariantNumeric: "tabular-nums" },
  strong: { fontWeight: weight.semibold },

  ink: { color: color.fg },
  soft: { color: color.fgSoft },
  muted: { color: color.fgMuted },
  faint: { color: color.fgFaint },
  accent: { color: color.accent },
  success: { color: color.success },
  warning: { color: color.warning },
  negative: { color: color.negative },
  info: { color: color.info },
});

export const gap = stylex.create({
  s0_5: { gap: space.s0_5 },
  s1: { gap: space.s1 },
  s1_5: { gap: space.s1_5 },
  s2: { gap: space.s2 },
  s2_5: { gap: space.s2_5 },
  s3: { gap: space.s3 },
  s4: { gap: space.s4 },
  s6: { gap: space.s6 },
});
