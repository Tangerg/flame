import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { color, leading, space, type } from "@/styles/tokens.stylex";
import { useScrollReach } from "./use-scroll-reach";

const styles = stylex.create({
  block: {
    maxHeight: "calc(var(--spacing) * 60)",
    overflowY: "auto",
    paddingInline: 0,
    paddingTop: space.s1,
    paddingBottom: 0,
    fontFamily: "var(--font-mono)",
    lineHeight: leading.body,
    color: color.fgMuted,
  },
  words: { minWidth: 0, whiteSpace: "pre-wrap", overflowWrap: "break-word" },
  soft: { color: color.fgSoft },
});

export function TextPreview({
  wrap,
  ink,
  children,
}: {
  wrap?: "words";
  ink?: "soft";
  children: ReactNode;
}) {
  return (
    <div
      {...useScrollReach()}
      {...stylex.props(
        styles.block,
        type.uiMd,
        wrap === "words" && styles.words,
        ink === "soft" && styles.soft,
      )}
    >
      {children}
    </div>
  );
}
