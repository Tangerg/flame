import * as stylex from "@stylexjs/stylex";
import type { ComponentPropsWithRef } from "react";
import { cn } from "@/lib/classNames";
import { radius, space, surface } from "@/styles/tokens.stylex";

/**
 * A plane, named by what it IS rather than assembled from fill, corner and cast.
 *
 * The product has four, and each one was being reached by cancelling parts of `card` at the
 * call site: `group` cancelled the fill and added a hairline, `request` and `prompt` cancelled
 * the corner, `prompt` also swapped the cast. Tailwind let all of that stand on source order,
 * so nothing said these were different planes — they read as a card with adjustments.
 *
 * StyleX ends that: a generated selector carries `:not(#\#)` specificity that no utility class
 * outranks, so every one of those overrides is silently dropped. Hence a closed set — three
 * orthogonal props would spell eight planes of which half mean nothing (a fill-less plane with
 * a popover cast), and a plane's identity is one decision, not three that happen to agree.
 *
 * `group` has no fill at all and states its edge, which is why it may never also carry a cast —
 * that is the double edge DESIGN.md §5 forbids. `request` and `prompt` sit inside the
 * transcript, where every block is a bubble.
 *
 * `card` and `request` state an edge TOO, because a fill alone does not make a plane. They read
 * from `--app-card-surface`, and a theme is free to set that to the same value as the canvas —
 * the light theme does, and Codex's light theme does the same thing, because in light mode a
 * raised plane has nowhere brighter to go. With `--shadow-surface-card: none` under every
 * shipped visual style, the fill was the only separation on offer and in light it was worth
 * zero: measured `rgb(255,255,255)` on `rgb(255,255,255)`, no border, no shadow, so an approval
 * had no boundary and its buttons read as loose page furniture.
 *
 * The edge is what does not depend on a delta this atom cannot see. A visual style that ever
 * gives `--shadow-surface-card` a real cast has to turn this off in the same change, or it is
 * the double edge again — `prompt` is the standing example, which is why it has no edge here.
 */
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
