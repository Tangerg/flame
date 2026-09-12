import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { color, corner, motion, space, surface, type, weight } from "@/styles/tokens.stylex";
import { ButtonPrimitive, type ButtonPrimitiveProps } from "@/ui/primitives";

type PillVariant = "outlined" | "solid" | "accent" | "danger";

const styles = stylex.create({
  base: {
    display: "inline-flex",
    alignItems: "center",
    gap: space.s1_5,
    fontFamily: "var(--font-sans)",
    fontWeight: weight.medium,
    transitionProperty: "background-color, color, scale",
    transitionDuration: motion.fast,
    transitionTimingFunction: "var(--ease-out)",
    // A disabled control does not answer a press.
    scale: {
      default: null,
      ":active": "var(--press-scale)",
      ':is(:disabled, [aria-disabled="true"]):active': 1,
    },
  },
  outlined: {
    borderWidth: "0.5px",
    borderStyle: "solid",
    borderColor: surface.field,
    color: { default: color.fgSoft, ":hover": color.fg },
    backgroundColor: { default: null, ":hover": surface.hover },
  },
  solid: {
    backgroundColor: { default: surface.ctaFill, ":hover": surface.ctaHover },
    color: color.ctaText,
  },
  accent: { backgroundColor: surface.ctaFill, color: color.ctaText },
  danger: {
    backgroundColor: { default: "transparent", ":hover": surface.negativeWash },
    color: color.negative,
    borderWidth: "0.5px",
    borderStyle: "solid",
    borderColor: color.negative,
  },
  // The pill's own tracking, against the UI step's: a capsule reads as a label rather than a
  // line of interface, and the negative tracking crowds it. Applied AFTER the type step, which
  // brings its own — here order is the only thing that decides.
  tracking: { letterSpacing: "var(--tracking-none)" },
  sm: { height: "calc(var(--spacing) * 6.5)", paddingInline: space.s3 },
  md: { height: space.s8, paddingInline: space.s3_5 },
});

const VARIANT = {
  outlined: styles.outlined,
  solid: styles.solid,
  accent: styles.accent,
  danger: styles.danger,
} as const;

const SIZE = { sm: [styles.sm, type.uiSm], md: [styles.md, type.uiMd] } as const;

type Props = Omit<ButtonPrimitiveProps, "children"> & {
  variant?: PillVariant;
  size?: keyof typeof SIZE;
  children: ReactNode;
};

export function PillButton({
  variant = "outlined",
  size = "md",
  className,
  children,
  ...rest
}: Props) {
  const styled = stylex.props(
    styles.base,
    corner.pill,
    SIZE[size],
    styles.tracking,
    VARIANT[variant],
  );
  return (
    <ButtonPrimitive {...rest} {...styled} className={cn(styled.className, className)}>
      {children}
    </ButtonPrimitive>
  );
}
