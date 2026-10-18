import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { color, motion, radius, space, surface, type } from "@flame/ui/tokens.stylex";
import { ButtonPrimitive, type ButtonPrimitiveProps } from "@flame/ui/primitives";
import { vocab } from "@flame/ui/vocabulary";

type TextButtonShape = "inline" | "row" | "link";

const styles = stylex.create({
  base: {
    display: "inline-flex",
    alignItems: "center",
    gap: space.s1_5,
    transitionProperty: "color, background-color, text-decoration-color",
    transitionDuration: motion.color,
    transitionTimingFunction: motion.easeState,
  },
  muted: {
    color: {
      default: color.fgMuted,
      ":hover:not(:disabled, [aria-disabled=true], [data-disabled])": color.fg,
    },
  },
  faint: {
    color: {
      default: color.fgFaint,
      ":hover:not(:disabled, [aria-disabled=true], [data-disabled])": color.fg,
    },
  },
  negative: {
    color: color.negative,
    textDecorationLine: "underline",
    textDecorationColor: {
      default: "transparent",
      ":hover:not(:disabled, [aria-disabled=true], [data-disabled])": "currentColor",
    },
  },
  inline: { borderRadius: radius.sm },
  row: {
    width: "100%",
    minHeight: "var(--control-height-sm)",
    borderRadius: radius.row,
    backgroundColor: {
      default: "transparent",
      ":hover:not(:disabled, [aria-disabled=true], [data-disabled])": surface.hover,
    },
  },
  link: {
    display: "inline",
    textDecorationLine: "underline",
    textDecorationColor: {
      default: "transparent",
      ":hover:not(:disabled, [aria-disabled=true], [data-disabled])": "currentColor",
    },
  },
});

const TONE = {
  muted: styles.muted,
  faint: styles.faint,
  accent: vocab.accent,
  negative: styles.negative,
} as const;

const SIZE = { xs: type.uiSm, sm: type.uiMd, md: type.uiMd } as const;

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
