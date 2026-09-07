import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { color, motion, radius, space, surface, type } from "@/styles/tokens.stylex";
import { ButtonPrimitive, type ButtonPrimitiveProps } from "@/ui/primitives";

/**
 * Text that acts, in the three shapes the product actually uses.
 *
 * `row` exists because this had no height of its own, so at the smallest UI size its box was
 * the text line — under the 24px target minimum. One call site said exactly that in a comment
 * and patched it; two others patched around the same gap without naming it. A control that
 * cannot be hit is the atom's defect, not each caller's.
 *
 * `row` states its minimum height, its corner and its hover wash, and NOT its padding: the
 * three rows inset differently and, under StyleX, a caller can only add what the atom leaves
 * unset. What the atom owns is what all three agree on.
 *
 * `link` is a reference into the workspace — mono, accent, underlined on hover. The underline
 * is always laid out and merely transparent, so it can transition with the colour instead of
 * appearing all at once.
 */
type TextButtonShape = "inline" | "row" | "link";

const styles = stylex.create({
  base: {
    display: "inline-flex",
    alignItems: "center",
    gap: space.s1_5,
    transitionProperty: "color, background-color, text-decoration-color",
    transitionDuration: motion.color,
  },
  muted: { color: { default: color.fgMuted, ":hover": color.fg } },
  faint: { color: { default: color.fgFaint, ":hover": color.fg } },
  accent: { color: color.accent },
  negative: { color: color.negative, opacity: { default: null, ":hover": 0.8 } },
  inline: {},
  row: {
    width: "100%",
    minHeight: "var(--control-height-sm)",
    borderRadius: radius.row,
    backgroundColor: { default: "transparent", ":hover": surface.hover },
  },
  link: {
    fontFamily: "var(--font-mono)",
    textDecorationLine: "underline",
    textDecorationColor: { default: "transparent", ":hover": "currentColor" },
  },
});

const TONE = {
  muted: styles.muted,
  faint: styles.faint,
  accent: styles.accent,
  negative: styles.negative,
} as const;

const SIZE = { xs: type.uiXs, sm: type.uiSm, md: type.uiMd } as const;

export type TextButtonProps = Omit<ButtonPrimitiveProps, "children"> & {
  tone?: keyof typeof TONE;
  size?: keyof typeof SIZE;
  shape?: TextButtonShape;
  children: ReactNode;
};

export function TextButton({
  tone = "muted",
  size = "md",
  shape = "inline",
  className,
  children,
  ...props
}: TextButtonProps) {
  const styled = stylex.props(styles.base, SIZE[size], TONE[tone], styles[shape]);
  return (
    <ButtonPrimitive {...props} {...styled} className={cn(styled.className, className)}>
      {children}
    </ButtonPrimitive>
  );
}
