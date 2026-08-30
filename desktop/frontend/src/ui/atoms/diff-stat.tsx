import { cn } from "@/lib/classNames";

/**
 * One strength, no dimmed variant: alpha on tinted text is contrast, and the 70% variant
 * this once had measured 3.08:1 at 12px — under the 4.5:1 the WCAG gate holds.
 */
export function DiffStat({
  added,
  removed,
  binary,
  className,
}: {
  added: number;
  removed: number;
  /**
   * Presence is how the caller says the content IS binary — a pair of zeroes would claim it
   * was touched and changed nothing. `string`, NOT `ReactNode`: the latter accepts `false`,
   * so a caller threading a boolean flag through compiles and takes this branch every row.
   */
  binary?: string;
  className?: string;
}) {
  const base = cn("shrink-0 items-center gap-1.5 font-mono tabular-nums", className);

  if (binary !== undefined) {
    return <span className={cn("inline-flex text-fg-faint", base)}>{binary}</span>;
  }
  // A dash rather than a blank: it holds the column so figures beside it stay in line.
  if (added === 0 && removed === 0) {
    return (
      <span aria-hidden className={cn("inline-flex text-fg-faint", base)}>
        —
      </span>
    );
  }

  return (
    <span className={cn("inline-flex", base)}>
      {added > 0 && <span className="text-success">+{added}</span>}
      {removed > 0 && <span className="text-negative">−{removed}</span>}
    </span>
  );
}
