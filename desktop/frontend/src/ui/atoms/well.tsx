import type { VariantProps } from "class-variance-authority";
import type { ReactNode } from "react";
import { cva } from "class-variance-authority";
import { cn } from "@/lib/classNames";

// A recessed block of text the system produced verbatim: a tool's output, a command awaiting
// approval, a JSON schema, a stack trace. Nine call sites had drawn it themselves in six
// spellings — two radii, four paddings, four type steps, three inks — so the same kind of
// block changed shape between the transcript and a workspace view.
//
// One shape now. `text-code` is the same 13px as `text-ui-sm` and drops the UI tracking that
// never belonged on mono, and the corner is `sm` because a well sits INSIDE a card whose own
// corner is `md`: the inner radius has to be the smaller one.
const styles = cva("m-0 rounded-sm bg-sunken px-3 py-2.5 font-mono text-code leading-relaxed", {
  variants: {
    ink: {
      soft: "text-fg-soft",
      // The thing being decided on, not reported — an approval's command line.
      strong: "text-fg font-medium",
    },
    wrap: {
      wrap: "whitespace-pre-wrap break-words",
      /** Machine text with no spaces to break at: a URL, a base64 blob, a long identifier. */
      anywhere: "whitespace-pre-wrap break-all",
      /** Columns that mean something — JSON, a diff, a table. Scrolls rather than reflows. */
      pre: "whitespace-pre",
    },
    /** Past this the block scrolls, so a long output cannot push the rest of the view away. */
    cap: {
      none: "",
      sm: "max-h-36 overflow-auto",
      md: "max-h-60 overflow-auto",
      lg: "max-h-80 overflow-auto",
    },
  },
  defaultVariants: { ink: "soft", wrap: "wrap", cap: "none" },
});

export type WellProps = VariantProps<typeof styles> & {
  /** `pre` unless the content is a single literal (`code`) or already elements (`div`). */
  as?: "pre" | "code" | "div";
  children: ReactNode;
  className?: string;
  "aria-live"?: "polite" | "assertive";
};

export function Well({ as: Element = "pre", ink, wrap, cap, className, ...props }: WellProps) {
  return <Element {...props} className={cn(styles({ ink, wrap, cap }), className)} />;
}
