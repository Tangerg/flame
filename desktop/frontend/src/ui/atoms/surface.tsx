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
 * `card` fills and is read from its value delta; `--shadow-surface-card` is the visual style's
 * hook and is `none` under the tool-window style. `group` has no fill at all and states its
 * edge, which is why it may never also carry a cast — that is the double edge DESIGN.md §5
 * forbids. `request` and `prompt` sit inside the transcript, where every block is a bubble.
 */
type SurfaceVariant = "card" | "group" | "request" | "prompt";

const styles = stylex.create({
  card: {
    borderRadius: radius.card,
    backgroundColor: surface.card,
    boxShadow: "var(--shadow-surface-card)",
  },
  group: {
    borderRadius: radius.card,
    borderWidth: "var(--control-edge-width)",
    borderStyle: "solid",
    borderColor: surface.field,
  },
  request: {
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
