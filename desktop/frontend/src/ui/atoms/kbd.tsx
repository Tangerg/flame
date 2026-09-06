import * as stylex from "@stylexjs/stylex";
import type { ComponentPropsWithoutRef } from "react";
import { cn } from "@/lib/classNames";
import { color, radius, space, surface, type } from "@/styles/tokens.stylex";

/**
 * A shortcut, in the two shapes the product actually shows it.
 *
 * `cap` is the key: a filled, padded plate sized to be tapped by the eye. `inline` is the
 * same shortcut spoken quietly inside a row that already has its own weight — no plate, no
 * padding, and the mono face that keeps a glyph sequence from re-flowing between rows.
 *
 * `inline` exists because a call site was cancelling nine of the cap's properties one by one
 * to get it. Under Tailwind that worked; under StyleX it does not — a generated selector
 * carries `:not(#\#)` specificity that no single utility class can outrank, so every one of
 * those overrides was silently discarded and the cap rendered anyway. The escape hatch was
 * never a design, only a leak that happened to work.
 */
type KbdVariant = "cap" | "inline";

const styles = stylex.create({
  base: {
    display: "inline-flex",
    alignItems: "center",
    justifyContent: "center",
    // A key cap is a label, not a target: it takes no pointer and no selection.
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
    fontWeight: 500,
    lineHeight: 1,
  },
  inline: {
    color: color.fgFaint,
    fontFamily: "var(--font-mono)",
    fontWeight: "var(--fw-regular)",
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
