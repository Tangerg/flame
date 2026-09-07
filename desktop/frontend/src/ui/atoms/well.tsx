import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { color, leading, radius, space, surface, type, weight } from "@/styles/tokens.stylex";

// A recessed block of text the system produced verbatim: a tool's output, a command awaiting
// approval, a JSON schema, a stack trace. Nine call sites had drawn it themselves in six
// spellings — two radii, four paddings, four type steps, three inks — so the same kind of
// block changed shape between the transcript and a workspace view.
//
// One shape now. `type.code` is the same 13px as `ui-sm` and drops the UI tracking that never
// belonged on mono, and the corner is `sm` because a well sits INSIDE a card whose own corner
// is `md`: the inner radius has to be the smaller one.
/** The surface itself. Exported for `TextArea variant="well"` — the editable face of the
 *  same block — so a well and its editor cannot drift apart. Inside the design system only:
 *  a call site that wants this surface asks for `<Well>`. */
export const WELL_SURFACE = stylex.create({
  face: {
    borderRadius: radius.sm,
    backgroundColor: surface.sunken,
    paddingInline: space.s3,
    paddingBlock: space.s2_5,
    fontFamily: "var(--font-mono)",
    lineHeight: leading.relaxed,
  },
});

const styles = stylex.create({
  base: { margin: 0 },
  // `<pre>` and `<div>` are already blocks; `<code>` is not, and a well is always one. The atom
  // derives this from `as` so no call site has to remember — and so a caller may still lay the
  // block's own contents out as a grid, which an unconditional `display` here would discard.
  block: { display: "block" },
  soft: { color: color.fgSoft },
  // The thing being decided on, not reported — an approval's command line.
  strong: { color: color.fg, fontWeight: weight.medium },
  wrap: { whiteSpace: "pre-wrap", overflowWrap: "break-word" },
  /** Machine text with no spaces to break at: a URL, a base64 blob, a long identifier. */
  anywhere: { whiteSpace: "pre-wrap", wordBreak: "break-all" },
  /** Columns that mean something — JSON, a diff, a table. Scrolls rather than reflows. */
  pre: { whiteSpace: "pre" },
  /** Past this the block scrolls, so a long output cannot push the rest of the view away. */
  capSm: { maxHeight: "calc(var(--spacing) * 36)", overflow: "auto" },
  capMd: { maxHeight: "calc(var(--spacing) * 60)", overflow: "auto" },
  capLg: { maxHeight: "calc(var(--spacing) * 80)", overflow: "auto" },
});

const CAP = { none: null, sm: styles.capSm, md: styles.capMd, lg: styles.capLg } as const;

export type WellProps = {
  ink?: "soft" | "strong";
  wrap?: "wrap" | "anywhere" | "pre";
  cap?: keyof typeof CAP;
  /** `pre` unless the content is a single literal (`code`) or already elements (`div`). */
  as?: "pre" | "code" | "div";
  children: ReactNode;
  className?: string;
  "aria-live"?: "polite" | "assertive";
};

export function Well({
  as: Element = "pre",
  ink = "soft",
  wrap = "wrap",
  cap = "none",
  className,
  ...props
}: WellProps) {
  const styled = stylex.props(
    styles.base,
    WELL_SURFACE.face,
    type.code,
    Element === "code" && styles.block,
    styles[ink],
    styles[wrap],
    CAP[cap],
  );
  return <Element {...props} {...styled} className={cn(styled.className, className)} />;
}
