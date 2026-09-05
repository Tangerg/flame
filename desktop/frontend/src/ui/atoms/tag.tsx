import type { VariantProps } from "class-variance-authority";
import type { ReactNode } from "react";
import { cva } from "class-variance-authority";
import { cn } from "@/lib/classNames";

// A literal, shown the way the system spells it: a cron expression, a transport name, a
// hook event, a revision, a scope slug. `<code>` because that is what it is.
//
// `Badge` is the sibling, not the same thing — a Badge names a STATE in the reader's
// language and takes the pill and a tone; a Tag carries a VALUE the reader must be able to
// copy back, so it is rectangular (DESIGN §6 gives `xs` to "anything that is really a tag")
// and never coloured. Nine hand-rolled spellings of this had drifted across the plugin
// layer, three of them rendering the same field two different ways in two views.
const styles = cva("shrink-0 rounded-xs bg-surface-2 px-1.5 font-mono", {
  variants: {
    size: {
      xs: "py-px text-ui-xs",
      sm: "py-0.5 text-ui-sm",
      // Inline in a sentence, where the tag has to sit on the prose it interrupts.
      md: "py-px text-ui-md",
    },
    // No `faint`. A Tag carries a value the reader has to be able to copy back, and
    // `--color-fg-faint` on this surface measures 3.99:1 in dark — below AA, for the one
    // element on the row whose whole job is to be read exactly. The surface already does the
    // quieting; `muted` is quiet AND legible, which is why it is the default.
    ink: {
      muted: "text-fg-muted",
      strong: "text-fg",
    },
  },
  defaultVariants: { size: "xs", ink: "muted" },
});

export type TagProps = VariantProps<typeof styles> & {
  /** Optional so a Tag can be handed to `<Trans components>` as the shape for a slot, where
   *  the translated sentence supplies the value. */
  children?: ReactNode;
  className?: string;
  title?: string;
};

export function Tag({ size, ink, className, children, title }: TagProps) {
  return (
    <code title={title} className={cn(styles({ size, ink }), className)}>
      {children}
    </code>
  );
}
