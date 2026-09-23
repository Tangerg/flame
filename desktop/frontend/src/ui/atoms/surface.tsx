import * as stylex from "@stylexjs/stylex";
import type { ComponentPropsWithRef } from "react";
import { cn } from "@/lib/classNames";
import { radius, space, surface } from "@/styles/tokens.stylex";

type SurfaceVariant = "card" | "group" | "request" | "prompt";

const edge = {
  borderWidth: "var(--control-edge-width)",
  borderStyle: "solid",
  borderColor: surface.field,
} as const;

const styles = stylex.create({
  card: {
    ...edge,
    borderRadius: radius.card,
    backgroundColor: surface.card,
    boxShadow: "var(--shadow-surface-card)",
  },
  group: {
    ...edge,
    borderRadius: radius.card,
  },
  request: {
    ...edge,
    borderRadius: radius.bubble,
    backgroundColor: surface.card,
    boxShadow: "var(--shadow-surface-card)",
  },
  prompt: {
    borderRadius: radius.bubble,
    backgroundColor: surface.card,
    boxShadow: "var(--shadow-popover)",
  },
  insetXs: { padding: space.s2 },
  insetSm: { padding: space.s3 },
  insetMd: { padding: space.s4 },
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
  const styled = stylex.props(styles[variant], INSET[inset]);
  return (
    <div {...props} {...styled} className={cn(styled.className, className)}>
      {children}
    </div>
  );
}
