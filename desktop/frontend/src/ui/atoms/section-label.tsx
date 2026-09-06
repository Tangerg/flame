import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { color, space, type } from "@/styles/tokens.stylex";

/**
 * A section's heading: its type, its ink, its truncation and where its trailing slot sits.
 *
 * NOT its inset. The atom used to carry `px-2 py-2`, and of nine call sites exactly one kept
 * it — the rest each cancelled or replaced it, because how deep a heading sits belongs to the
 * container it sits in, not to the heading. Under Tailwind those overrides worked and the
 * disagreement stayed invisible; under StyleX they were discarded, which is how it surfaced.
 */
const styles = stylex.create({
  row: {
    display: "flex",
    minWidth: 0,
    alignItems: "center",
    gap: space.s2,
    fontFamily: "var(--font-sans)",
    fontWeight: 500,
    lineHeight: "var(--leading-tight)",
    color: color.fgFaint,
  },
  label: { minWidth: 0, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" },
  // Pushed to the far end and exempt from the label's own casing and tracking: what rides
  // here is a count or a control, not more heading.
  trailing: {
    marginInlineStart: "auto",
    display: "flex",
    flexShrink: 0,
    alignItems: "center",
    gap: space.s1_5,
    textTransform: "none",
    letterSpacing: "normal",
  },
});

export function SectionLabel({
  children,
  trailing,
  className,
}: {
  children: ReactNode;
  trailing?: ReactNode;
  className?: string;
}) {
  const row = stylex.props(styles.row, type.uiXs);
  return (
    <div {...row} className={cn(row.className, className)}>
      <span {...stylex.props(styles.label)}>{children}</span>
      {trailing != null && <span {...stylex.props(styles.trailing)}>{trailing}</span>}
    </div>
  );
}
