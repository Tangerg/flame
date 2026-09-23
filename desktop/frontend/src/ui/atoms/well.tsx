import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { color, leading, radius, space, surface, type, weight } from "@/styles/tokens.stylex";
import { useScrollReach } from "./use-scroll-reach";

export const WELL_SURFACE = stylex.create({
  face: {
    borderRadius: radius.sm,
    backgroundColor: surface.sunken,
    paddingInline: space.s3,
    paddingBlock: space.s2_5,
    fontFamily: "var(--font-mono)",
    lineHeight: leading.relaxed,
  },
});

const styles = stylex.create({
  base: { margin: 0 },
  block: { display: "block" },
  soft: { color: color.fgSoft },
  strong: { color: color.fg, fontWeight: weight.medium },
  wrap: { whiteSpace: "pre-wrap", overflowWrap: "break-word" },
  anywhere: { whiteSpace: "pre-wrap", wordBreak: "break-all" },
  pre: { whiteSpace: "pre" },
  capSm: { maxHeight: "calc(var(--spacing) * 36)", overflow: "auto" },
  capMd: { maxHeight: "calc(var(--spacing) * 60)", overflow: "auto" },
  capLg: { maxHeight: "calc(var(--spacing) * 80)", overflow: "auto" },
});

const CAP = { none: null, sm: styles.capSm, md: styles.capMd, lg: styles.capLg } as const;

export type WellProps = {
  ink?: "soft" | "strong";
  wrap?: "wrap" | "anywhere" | "pre";
  cap?: keyof typeof CAP;
  as?: "pre" | "code" | "div";
  children: ReactNode;
  className?: string;
  "aria-live"?: "polite" | "assertive";
};

export function Well({
  as: Element = "pre",
  ink = "soft",
  wrap = "wrap",
  cap = "none",
  className,
  ...props
}: WellProps) {
  const styled = stylex.props(
    styles.base,
    WELL_SURFACE.face,
    type.code,
    Element === "code" && styles.block,
    styles[ink],
    styles[wrap],
    CAP[cap],
  );
  return (
    <Element
      {...props}
      {...useScrollReach()}
      {...styled}
      className={cn(styled.className, className)}
    />
  );
}
