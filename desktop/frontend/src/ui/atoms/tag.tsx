import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { color, radius, space, surface, type } from "@/styles/tokens.stylex";

const styles = stylex.create({
  base: {
    flexShrink: 0,
    borderRadius: radius.xs,
    backgroundColor: surface.surface2,
    paddingInline: space.s1_5,
    fontFamily: "var(--font-mono)",
  },
  xs: { paddingBlock: "1px" },
  sm: { paddingBlock: space.s0_5 },
  md: { paddingBlock: "1px" },
  muted: { color: color.fgMuted },
  strong: { color: color.fg },
});

type TagSize = "xs" | "sm" | "md";
type TagInk = "muted" | "strong";

const SIZE_TYPE: Record<TagSize, (typeof type)[keyof typeof type]> = {
  xs: type.uiXs,
  sm: type.uiSm,
  md: type.uiMd,
};

export type TagProps = {
  size?: TagSize;
  ink?: TagInk;
  children?: ReactNode;
  className?: string;
  title?: string;
};

export function Tag({ size = "xs", ink = "muted", className, children, title }: TagProps) {
  const styled = stylex.props(styles.base, styles[size], SIZE_TYPE[size], styles[ink]);
  return (
    <code title={title} {...styled} className={cn(styled.className, className)}>
      {children}
    </code>
  );
}
