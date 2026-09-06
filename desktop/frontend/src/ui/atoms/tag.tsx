import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { color, radius, space, surface, type } from "@/styles/tokens.stylex";

// A literal, shown the way the system spells it: a cron expression, a transport name, a
// hook event, a revision, a scope slug. `<code>` because that is what it is.
//
// `Badge` is the sibling, not the same thing — a Badge names a STATE in the reader's
// language and takes the pill and a tone; a Tag carries a VALUE the reader must be able to
// copy back, so it is rectangular (DESIGN §6 gives `xs` to "anything that is really a tag")
// and never coloured. Nine hand-rolled spellings of this had drifted across the plugin
// layer, three of them rendering the same field two different ways in two views.
const styles = stylex.create({
  base: {
    flexShrink: 0,
    borderRadius: radius.xs,
    backgroundColor: surface.surface2,
    paddingInline: space.s1_5,
    fontFamily: "var(--font-mono)",
  },
  xs: { paddingBlock: "1px" },
  sm: { paddingBlock: space.s0_5 },
  // Inline in a sentence, where the tag has to sit on the prose it interrupts.
  md: { paddingBlock: "1px" },
  // No `faint`. A Tag carries a value the reader has to be able to copy back, and
  // `--color-fg-faint` on this surface measures 3.99:1 in dark — below AA, for the one
  // element on the row whose whole job is to be read exactly. The surface already does the
  // quieting; `muted` is quiet AND legible, which is why it is the default.
  muted: { color: color.fgMuted },
  strong: { color: color.fg },
});

type TagSize = "xs" | "sm" | "md";
type TagInk = "muted" | "strong";

const SIZE_TYPE: Record<TagSize, (typeof type)[keyof typeof type]> = {
  xs: type.uiXs,
  sm: type.uiSm,
  md: type.uiMd,
};

export type TagProps = {
  size?: TagSize;
  ink?: TagInk;
  /** Optional so a Tag can be handed to `<Trans components>` as the shape for a slot, where
   *  the translated sentence supplies the value. */
  children?: ReactNode;
  className?: string;
  title?: string;
};

export function Tag({ size = "xs", ink = "muted", className, children, title }: TagProps) {
  const styled = stylex.props(styles.base, styles[size], SIZE_TYPE[size], styles[ink]);
  return (
    <code title={title} {...styled} className={cn(styled.className, className)}>
      {children}
    </code>
  );
}
