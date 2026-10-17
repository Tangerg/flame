import * as stylex from "@stylexjs/stylex";
import type { ComponentPropsWithRef } from "react";
import { cn } from "@/lib/classNames";
import { CARD_SURFACE } from "@flame/ui";
import { layout, radius, space, surface } from "@flame/ui/tokens.stylex";

type SurfaceVariant = "card" | "prompt";

const styles = stylex.create({
  base: { minWidth: 0 },
  prompt: {
    borderRadius: radius.bubble,
    backgroundColor: surface.card,
    boxShadow: "var(--shadow-popover)",
  },
  insetXs: { padding: space.s2 },
  insetSm: { padding: space.s3 },
  insetMd: { padding: layout.cardPadding },
});

const INSET = { none: null, xs: styles.insetXs, sm: styles.insetSm, md: styles.insetMd } as const;

export type SurfaceProps = ComponentPropsWithRef<"div"> & {
  variant?: SurfaceVariant;
  inset?: keyof typeof INSET;
};

export function Surface({
  variant = "card",
  inset = "md",
  className,
  children,
  ...props
}: SurfaceProps) {
  const styled = stylex.props(
    styles.base,
    variant === "card" ? CARD_SURFACE.face : styles.prompt,
    INSET[inset],
  );
  return (
    <div {...props} {...styled} className={cn(styled.className, className)}>
      {children}
    </div>
  );
}
