import * as stylex from "@stylexjs/stylex";
import { cn } from "@/lib/classNames";
import { color, space } from "@/styles/tokens.stylex";

const styles = stylex.create({
  row: {
    display: "inline-flex",
    alignItems: "center",
    flexShrink: 0,
    gap: space.s1_5,
    fontFamily: "var(--font-mono)",
  },
  quiet: { color: color.fgFaint },
  added: { color: color.success },
  removed: { color: color.negative },
});

export function DiffStat({
  added,
  removed,
  binary,
  className,
}: {
  added: number;
  removed: number;
  binary?: string;
  className?: string;
}) {
  const quiet = stylex.props(styles.row, styles.quiet);
  if (binary !== undefined) {
    return (
      <span {...quiet} className={cn(quiet.className, className)}>
        {binary}
      </span>
    );
  }
  // A measured change of nothing, said once. Absent counts are `undefined` upstream and never
  // reach here, so a dash means "measured, and it was zero" rather than "not measured".
  if (added === 0 && removed === 0) {
    return (
      <span aria-hidden {...quiet} className={cn(quiet.className, className)}>
        —
      </span>
    );
  }

  const row = stylex.props(styles.row);
  return (
    <span {...row} className={cn(row.className, className)}>
      {added > 0 && <span {...stylex.props(styles.added)}>+{added}</span>}
      {removed > 0 && <span {...stylex.props(styles.removed)}>−{removed}</span>}
    </span>
  );
}
