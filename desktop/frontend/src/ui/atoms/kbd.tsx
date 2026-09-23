import * as stylex from "@stylexjs/stylex";
import type { ComponentPropsWithoutRef } from "react";
import { cn } from "@/lib/classNames";
import { color, radius, space, surface, type, weight } from "@/styles/tokens.stylex";

type KbdVariant = "cap" | "inline";

const styles = stylex.create({
  base: {
    display: "inline-flex",
    alignItems: "center",
    justifyContent: "center",
    pointerEvents: "none",
    userSelect: "none",
  },
  cap: {
    height: space.s5,
    minWidth: space.s5,
    paddingInline: space.s1,
    borderRadius: radius.step2xs,
    backgroundColor: surface.sunken,
    color: color.fgMuted,
    fontFamily: "var(--font-sans)",
    fontWeight: weight.medium,
    lineHeight: 1,
  },
  inline: {
    color: color.fgFaint,
    fontFamily: "var(--font-mono)",
    fontWeight: weight.regular,
    lineHeight: 1,
  },
});

const SIZE: Record<KbdVariant, ReturnType<typeof stylex.create>[string]> = {
  cap: type.uiSm,
  inline: type.ui2xs,
};

export function Kbd({
  variant = "cap",
  className,
  ...props
}: ComponentPropsWithoutRef<"kbd"> & { variant?: KbdVariant }) {
  const styled = stylex.props(styles.base, styles[variant], SIZE[variant]);
  return <kbd {...props} {...styled} className={cn(styled.className, className)} />;
}
